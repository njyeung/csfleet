package proxy

import (
	"net/netip"
	"sync"
	"time"
)

type verdict int

const (
	accept verdict = iota // accept unmodified (unmanaged port)
	dnat                  // rewrite destination to backend, then accept
	drop                  // NF_DROP
)

type backend struct {
	ip  netip.Addr
	gen uint64
}

type pool struct {
	backends []backend
	rr       uint64 // round-robin cursor
}

type session struct {
	backend  netip.Addr
	gen      uint64
	lastSeen time.Time
}

// router is the in-memory routing state. It holds no database or netlink
// handles: the manager mutates it through the Proxy's exported methods and the
// NFQUEUE handler reads it per packet. Every decision is a pure function of this
// state, which makes the session/generation logic unit-testable without
// privileges. State is rebuilt from scratch on each startup, so there is nothing
// to reconcile after a crash.
type router struct {
	mu sync.Mutex

	// key: <dest port> value: <cluster/standalone server>
	pools map[uint16]*pool

	// key: <client source addr:port> value: session
	sessions map[netip.AddrPort]session

	// monotonic counter used for picking the next backend's gen
	gen uint64
}

func newRouter() *router {
	return &router{
		pools:    map[uint16]*pool{},
		sessions: map[netip.AddrPort]session{},
	}
}

// addBackend ensures a pool exists for the external port and appends a backend
// carrying a fresh generation, so a same-IP restart is distinguishable from the
// instance a client was already talking to. The generation doubles as the
// backend's nft map key (the mark): unique per add, so the kernel DNAT picks the
// right target and a restarted backend gets a new key.
//
// It returns the new generation. If a backend for ip was already present, its
// generation is bumped in place; replaced is true and oldGen is the generation
// whose stale map element the caller must delete.
func (r *router) addBackend(port uint16, ip netip.Addr) (newGen, oldGen uint64, replaced bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gen++
	p := r.pools[port]
	if p == nil {
		p = &pool{}
		r.pools[port] = p
	}
	for i := range p.backends {
		if p.backends[i].ip == ip {
			oldGen = p.backends[i].gen
			p.backends[i].gen = r.gen
			return r.gen, oldGen, true
		}
	}
	p.backends = append(p.backends, backend{ip: ip, gen: r.gen})
	return r.gen, 0, false
}

// removeBackend drops a backend from its pool but leaves the (possibly empty)
// pool in place, so the port stays managed: its packets are dropped, not
// accepted unmodified, until a backend returns or the port is unmanaged.
//
// It returns the removed backend's generation (its nft map key) so the caller
// can delete the matching map element. ok is false if no such backend existed.
// empty reports whether the pool has no backends left after the removal, so a
// caller reassigning a port (rebind) can decide to unmanage the now-drained port.
func (r *router) removeBackend(port uint16, ip netip.Addr) (gen uint64, ok, empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := r.pools[port]
	if p == nil {
		return 0, false, false
	}
	for i := range p.backends {
		if p.backends[i].ip == ip {
			gen = p.backends[i].gen
			p.backends = append(p.backends[:i], p.backends[i+1:]...)
			return gen, true, len(p.backends) == 0
		}
	}
	return 0, false, len(p.backends) == 0
}

// unmanage removes a port entirely. Afterwards its packets are accepted
// unmodified (no longer a managed port). The manager calls this when a cluster
// or standalone server definition is deleted, not merely stopped.
func (r *router) unmanage(port uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pools, port)
}

// route decides the fate of a ct-state-new first packet from client destined for destPort.
//
// On dnat it returns the chosen backend's mark, ie the nft @pool map key the
// handler stamps on the packet so the kernel DNATs it to that backend. The mark
// is the backend's generation (unique per add), so it both selects the right
// target and lets a same-IP restart be told apart from the instance a client was
// already talking to. The mark is meaningless for accept and drop.
//
// It records new assignments and refreshes lastSeen on accepted packets, but never deletes or refreshes on a drop.
// That is what lets an orphan age out from its last legitimate activity.
//
// We handle sweep orphans on lastSeen > 10m > conntrackUDPTimeout (30s). The invariant is that
// lastSeen > conntrackUDPTimeout - so there is never a case where an entry is forgotten while
// its client can still send a packet we'd misclassify.
func (r *router) route(client netip.AddrPort, destPort uint16, now time.Time) (mark uint32, v verdict) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := r.pools[destPort]
	if p == nil {
		return 0, accept // unmanaged port
	}

	if s, ok := r.sessions[client]; ok {
		if b, ok := p.find(s.backend); ok && b.gen == s.gen {
			s.lastSeen = now
			r.sessions[client] = s
			return uint32(s.gen), dnat // same instance
		}
		return 0, drop // orphan: backend gone or generation stale
	}

	if len(p.backends) == 0 {
		return 0, drop // managed port, all backends down
	}

	b := p.backends[p.rr%uint64(len(p.backends))]
	p.rr++
	r.sessions[client] = session{backend: b.ip, gen: b.gen, lastSeen: now}
	return uint32(b.gen), dnat // genuinely new client
}

// sweep removes sessions whose last accepted packet is older than ttl. Because
// lastSeen is refreshed only on an accepted packet, an orphan being repeatedly
// dropped ages out from its last legitimate activity.
func (r *router) sweep(ttl time.Duration, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c, s := range r.sessions {
		if now.Sub(s.lastSeen) > ttl {
			delete(r.sessions, c)
		}
	}
}

func (p *pool) find(ip netip.Addr) (backend, bool) {
	for _, b := range p.backends {
		if b.ip == ip {
			return b, true
		}
	}
	return backend{}, false
}
