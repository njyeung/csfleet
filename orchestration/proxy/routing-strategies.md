# Routing Strategies (future work)

The proxy currently routes new clients to a cluster's backends with **round-robin**
(`router.route` → `pool.rr`). This document captures the other strategies a CS2
fleet operator would want, why they matter, and the shape they'd take — so the
work is scoped before anyone starts it. None of this is built yet.

## What a cluster heuristic can and can't be

Three properties of the design constrain the whole menu. They are the reason some
"obvious" ideas (per-player map choice, live rebalancing) are *not* on the list.

1. **Placement, not rebalancing.** A strategy runs exactly once — on the first
   packet of a new flow — and conntrack makes the assignment sticky for the life
   of the connection. There is no way to move a connected player to another
   backend without forcing a reconnect. Every strategy here decides *where a new
   client lands*, never *where an existing client should now be*.

2. **The only inputs are the first UDP packet + proxy-local state.** That packet
   carries a source IP/port and a destination port — nothing else. There is no
   per-player preference signal (no "I want surf_utopia"). So **map choice is not
   a cluster heuristic**: a cluster is a pool of *interchangeable* servers, and
   the heuristic only picks which interchangeable one. Letting a player choose a
   specific map is a separate **standalone server** the UI connects to directly.

3. **The proxy knows session counts, not true player counts.** The session map
   tells us how many flows we routed to each backend; it does not know about
   bots, spectators, or players who joined out-of-band. Placement strategies
   should run on proxy-local data and stay dumb. Decisions that need real player
   counts (e.g. scaling a cluster down) belong to the manager, which can RCON.

## The strategies

Round-robin (current) is the neutral default: even spread, order-stable. The
useful additions, mapped to the workloads CSFleet targets:

| Strategy | Behavior | Primary workload |
|---|---|---|
| Round-robin *(built)* | even spread, order-stable | generic default, inspect |
| **Least-connections** | pick the backend with the fewest active sessions | **surf, inspect** — tickrate/perf-sensitive; isolates players; drains evenly |
| **Fill-first / pack** | cram onto the earliest non-full backend, spill to the next only at capacity | **community deathmatch** (servers feel alive) and **scale-down** (idle servers go empty so the manager can kill them) |
| **Source-IP sticky** | same client IP re-lands the same backend across reconnects | surf (keeps a player's timer/checkpoint server warm), inspect (re-lands the same box) |
| Weighted *(modifier)* | a bigger box takes proportionally more clients | mixed-hardware fleets |

The interesting tension is **deathmatch vs surf/inspect**: DM wants
*concentration* (an empty DM server is dead, so fill-first keeps games populated),
while surf and inspect want *spread* (isolation = better tickrate and
responsiveness, so least-connections). They are opposite goals — which is exactly
why the strategy has to be a per-cluster setting rather than a global policy.

Plain random is intentionally omitted: weighted-random subsumes it.

### Least-connections

Pick the in-pool backend with the fewest active sessions assigned to it. "Active"
means a session whose backend is still in the pool *and* whose generation is
current — counting this way at pick time naturally excludes orphans (a dead
backend's sessions stop counting the moment it leaves the pool) without any extra
bookkeeping. Cost is O(active sessions) per *new* connection, which is negligible
(new connections are rare; the session map is small). If that ever shows up in a
profile, switch to a per-backend counter maintained on assign + sweep.

### Fill-first / pack

Walk the backends in order and pick the first one below capacity; only spill to
the next when the current one is full. Two payoffs:

- **DM feels alive.** Players are concentrated instead of scattered one-per-server.
- **Scale-down becomes possible.** Trailing backends stay empty, so the manager
  can safely tear them down (and bring them back under load).

Capacity (`maxplayers`) comes from the server definition and is fed in by the
manager via `AddBackend` — the proxy does not learn it on its own. Current load
is the proxy session count (point 3 above): good enough for *placement*. Strict
"never overfill" is not guaranteed — CS2 itself rejects a player past
`maxplayers`, and accurate teardown decisions are the manager's job with real
counts.

### Source-IP sticky

A modifier, not a standalone strategy: it composes with any base strategy
("least-connections, but sticky"). On a new connection, if the client's source IP
maps to a backend still in the pool, reuse it; otherwise fall through to the base
strategy and record the mapping. This gives reconnect affinity — a surfer who
drops and reconnects from a fresh ephemeral port still lands on the server holding
their warmed state.

Caveat: it keys on source IP, so multiple players behind one NAT clump onto the
same backend. For CS2 (one Steam account = one client) this is usually fine, but
it makes the spread lumpier than the base strategy alone.

### Weighted (modifier)

A per-backend `weight` biases round-robin or random so a more powerful host takes
proportionally more clients. Also fed via `AddBackend` from the server
definition. Lowest priority of the set — only matters on heterogeneous hardware.

## Proposed shape

A `Strategy` enum on the pool, set per cluster by the manager, plus per-backend
fields for the inputs the strategies need:

```go
type Strategy uint8

const (
	RoundRobin Strategy = iota // default
	LeastConn
	FillFirst
)

type backend struct {
	ip       netip.Addr
	gen      uint64
	weight   int // weighted variants; 0/1 = equal
	capacity int // maxplayers, fed by the manager; 0 = unbounded
}

type pool struct {
	strategy Strategy
	sticky   bool // source-IP affinity modifier, composes with any strategy
	backends []backend
	rr       uint64
}
```

`route`'s genuinely-new-client branch (the `p.rr` pick today) becomes a
`p.pick(r, client)` that switches on `strategy` (and honors `sticky` first).
`AddBackend` grows to carry `weight`/`capacity`; the rest of `route` — the
session/generation/orphan logic — is unchanged, since all of this only affects
*which backend a brand-new client gets*.

## Out of scope (and why)

- **Per-player map selection** — no signal in the first packet; use standalone
  servers the UI routes to directly (constraint 2).
- **Live rebalancing of connected players** — impossible without a forced
  reconnect (constraint 1).
- **True player-count-aware placement** — the proxy stays dumb; counts that need
  RCON live in the manager (constraint 3).
