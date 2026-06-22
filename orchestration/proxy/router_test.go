package proxy

import (
	"net/netip"
	"testing"
	"time"
)

func ip(s string) netip.Addr                { return netip.MustParseAddr(s) }
func cli(s string, p uint16) netip.AddrPort { return netip.AddrPortFrom(ip(s), p) }

func TestRouteRoundRobinsNewClients(t *testing.T) {
	r := newRouter()
	r.addBackend(27100, ip("172.30.0.11"))
	r.addBackend(27100, ip("172.30.0.12"))

	now := time.Now()
	b1, v1 := r.route(cli("10.0.0.1", 5000), 27100, now)
	b2, v2 := r.route(cli("10.0.0.2", 5000), 27100, now)
	if v1 != dnat || v2 != dnat {
		t.Fatalf("verdicts = %v, %v; want dnat, dnat", v1, v2)
	}
	if b1 == b2 {
		t.Fatalf("round-robin sent both new clients to %v", b1)
	}
}

func TestRouteEstablishedStaysOnSameInstance(t *testing.T) {
	r := newRouter()
	r.addBackend(27100, ip("172.30.0.11"))
	c := cli("10.0.0.1", 5000)

	first, _ := r.route(c, 27100, time.Now())
	again, v := r.route(c, 27100, time.Now())
	if v != dnat || again != first {
		t.Fatalf("re-route = (%v, %v); want (%v, dnat)", again, v, first)
	}
}

func TestRouteOrphanDroppedWhenBackendGone(t *testing.T) {
	r := newRouter()
	addr := ip("172.30.0.11")
	r.addBackend(27100, addr)
	c := cli("10.0.0.1", 5000)
	r.route(c, 27100, time.Now())

	r.removeBackend(27100, addr)
	if _, v := r.route(c, 27100, time.Now()); v != drop {
		t.Fatalf("orphan verdict = %v; want drop", v)
	}
	if _, ok := r.sessions[c]; !ok {
		t.Fatal("session deleted on drop — an orphan could slip back onto the fast path")
	}
}

func TestRouteOrphanDroppedOnGenerationMismatch(t *testing.T) {
	r := newRouter()
	addr := ip("172.30.0.11")
	r.addBackend(27100, addr)
	c := cli("10.0.0.1", 5000)
	r.route(c, 27100, time.Now())

	// same-IP restart: remove then re-add bumps the generation (the new mark)
	r.removeBackend(27100, addr)
	newGen, _, _ := r.addBackend(27100, addr)

	if _, v := r.route(c, 27100, time.Now()); v != drop {
		t.Fatalf("stale-generation verdict = %v; want drop", v)
	}
	// a genuinely new client is still admitted to the fresh instance — and gets
	// its mark, not the stale one.
	if m, v := r.route(cli("10.0.0.9", 5000), 27100, time.Now()); v != dnat || m != uint32(newGen) {
		t.Fatalf("new client = (mark %d, %v); want (mark %d, dnat)", m, v, uint32(newGen))
	}
}

func TestRouteEmptyManagedPoolDrops(t *testing.T) {
	r := newRouter()
	r.addBackend(27100, ip("172.30.0.11"))
	r.removeBackend(27100, ip("172.30.0.11"))
	if _, v := r.route(cli("10.0.0.1", 5000), 27100, time.Now()); v != drop {
		t.Fatalf("empty managed pool verdict = %v; want drop", v)
	}
}

func TestRouteUnmanagedPortAccepts(t *testing.T) {
	r := newRouter()
	if _, v := r.route(cli("10.0.0.1", 5000), 53, time.Now()); v != accept {
		t.Fatalf("unmanaged port verdict = %v; want accept", v)
	}
}

func TestUnmanageReleasesPort(t *testing.T) {
	r := newRouter()
	r.addBackend(27100, ip("172.30.0.11"))
	r.removeBackend(27100, ip("172.30.0.11"))
	if _, v := r.route(cli("10.0.0.1", 5000), 27100, time.Now()); v != drop {
		t.Fatal("want drop while the port is still managed but empty")
	}
	r.unmanage(27100)
	if _, v := r.route(cli("10.0.0.1", 6000), 27100, time.Now()); v != accept {
		t.Fatal("want accept after unmanage")
	}
}

func TestSweepAgesOutFromLastAccept(t *testing.T) {
	r := newRouter()
	r.addBackend(27100, ip("172.30.0.11"))
	c := cli("10.0.0.1", 5000)
	start := time.Now()
	r.route(c, 27100, start)

	r.sweep(10*time.Minute, start.Add(5*time.Minute))
	if _, ok := r.sessions[c]; !ok {
		t.Fatal("session swept before its TTL")
	}
	r.sweep(10*time.Minute, start.Add(11*time.Minute))
	if _, ok := r.sessions[c]; ok {
		t.Fatal("stale session not swept after its TTL")
	}
}
