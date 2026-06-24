// Package proxy is the UDP reverse proxy for the fleet.
package proxy

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"time"

	"sync"

	nfqueue "github.com/florianl/go-nfqueue"
	"github.com/google/nftables"
	"github.com/ti-mo/conntrack"
	"golang.org/x/sys/unix"
)

const (
	// Every backend container listens on this internal port; only its bridge
	// address varies, so DNAT only ever rewrites the destination IP and this.
	backendPort uint16 = 27015

	// UDP conntrack timeout. Short, because game flows are steady and a quick
	// expiry returns a silently-gone client to the handler as ct state new.
	conntrackUDPTimeout = 30 // seconds

	sweepInterval = 30 * time.Minute
	sessionTTL    = 10 * time.Minute
)

type Config struct {
	Table       string // nft table name (e.g. "csfleet")
	Subnet      string // bridge subnet for the DOCKER-USER accept rule
	QueueNum    uint16 // NFQUEUE number (default 0)
	QueueMaxLen uint32 // NFQUEUE max length (default 0xffff)
}

type Proxy struct {
	cfg Config
	r   *router

	// opMu serializes the backend-mutation entrypoints (AddBackend, RemoveBackend,
	// FlushConntrack) so they can be called concurrently.
	opMu sync.Mutex

	// nft handles retained from installTable so AddBackend/RemoveBackend can add
	// and delete @pool map elements (mark -> backend) without rebuilding anything.
	nftTable *nftables.Table
	nftPool  *nftables.Set

	nfq    *nfqueue.Nfqueue
	ct     *conntrack.Conn
	cancel context.CancelFunc
}

func New(cfg Config) *Proxy {
	if cfg.QueueMaxLen == 0 {
		cfg.QueueMaxLen = 0xffff
	}
	return &Proxy{cfg: cfg, r: newRouter()}
}

// Start installs the static kernel plumbing and begins serving NFQUEUE. It is
// the proxy's startup lifecycle hook
//
// The manager calls it once before registering any backends.
// If any step fails, we rolled back.
//
// Stop is the pair.
func (p *Proxy) Start(ctx context.Context) error {
	if err := setConntrackUDPTimeouts(conntrackUDPTimeout); err != nil {
		return err
	}
	if err := p.installTable(); err != nil {
		return err
	}
	if err := installDockerUser(p.cfg.Subnet, backendPort); err != nil {
		deleteTable(p.cfg.Table)
		return err
	}

	ct, err := conntrack.Dial(nil)
	if err != nil {
		p.teardownKernel()
		return fmt.Errorf("conntrack dial: %w", err)
	}
	p.ct = ct

	nfq, err := nfqueue.Open(&nfqueue.Config{
		NfQueue:      p.cfg.QueueNum,
		MaxQueueLen:  p.cfg.QueueMaxLen,
		MaxPacketLen: 0xffff,
		Copymode:     nfqueue.NfQnlCopyPacket,
		AfFamily:     unix.AF_INET,
	})
	if err != nil {
		ct.Close()
		p.ct = nil
		p.teardownKernel()
		return fmt.Errorf("nfqueue open: %w", err)
	}
	p.nfq = nfq

	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	if err := nfq.RegisterWithErrorFunc(runCtx, p.onPacket, p.onError); err != nil {
		cancel()
		nfq.Close()
		ct.Close()
		p.nfq, p.ct = nil, nil
		p.teardownKernel()
		return fmt.Errorf("nfqueue register: %w", err)
	}

	go p.sweepLoop(runCtx)

	log.Printf("[proxy] serving NFQUEUE %d, table ip %s", p.cfg.QueueNum, p.cfg.Table)
	return nil
}

// Stop is the shutdown lifecycle hook. It tears down everything Start created in
// reverse
//
// Stop serving NFQUEUE, then remove the DOCKER-USER rule and the nft
// table.
//
// Conntrack entries expire on their own once there is nothing to route to.
func (p *Proxy) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.nfq != nil {
		p.nfq.Close()
	}
	if p.ct != nil {
		p.ct.Close()
	}
	p.teardownKernel()
	log.Printf("[proxy] stopped")
}

func (p *Proxy) teardownKernel() {
	if err := removeDockerUser(p.cfg.Subnet, backendPort); err != nil {
		log.Printf("[proxy] remove DOCKER-USER rule: %v", err)
	}
	deleteTable(p.cfg.Table)
}

// AddBackend registers a live backend under an external port: a cluster port
// (one of several backends) or a standalone server's port (the sole backend).
// It bumps the generation counter, so a same-IP restart is distinguishable from
// the instance a client was already talking to.
//
// IMPORTANT: The manager MUST perform these in order: Flush Conntrack -> Start Server -> Add Backend
func (p *Proxy) AddBackend(port uint16, ip string) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("add backend: bad ip %q: %w", ip, err)
	}
	addr = addr.Unmap()

	newGen, oldGen, replaced := p.r.addBackend(port, addr)
	// the lifecycle normally removes the hold ip first, but we drop the
	// stale element before installing the new one anyway
	if replaced {
		if err := p.delPoolElement(uint32(oldGen)); err != nil {
			log.Printf("[proxy] add backend %s: stale element: %v", ip, err)
		}
	}
	if err := p.addPoolElement(uint32(newGen), addr); err != nil {
		p.r.removeBackend(port, addr)
		return fmt.Errorf("add backend %s: %w", ip, err)
	}
	log.Printf("[proxy] backend %s up on port %d (gen %d)", ip, port, newGen)
	return nil
}

// RemoveBackend drops a backend from its port's pool. The pool is kept even if
// it becomes empty, so the port stays managed and its packets are dropped.
//
// IMPORTANT: The manager MUST perform these in order: Remove Backend -> Flush Conntrack
// Stopping the server is the least time-sensitive, it can technically happen anywhere, but the
// cleanest order is right after Flush Conntrack
func (p *Proxy) RemoveBackend(port uint16, ip string) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("remove backend: bad ip %q: %w", ip, err)
	}
	if gen, ok := p.r.removeBackend(port, addr.Unmap()); ok {
		if err := p.delPoolElement(uint32(gen)); err != nil {
			log.Printf("[proxy] remove backend %s: element: %v", ip, err)
		}
	}
	log.Printf("[proxy] backend %s down on port %d", ip, port)
	return nil
}

// Unmanage stops managing an external port entirely; afterwards its packets are
// accepted unmodified rather than dropped.
//
// The manager calls this when a cluster is deleted or standalone server definition is deleted.
func (p *Proxy) Unmanage(port uint16) {
	p.r.unmanage(port)
	log.Printf("[proxy] port %d unmanaged", port)
}

func (p *Proxy) sweepLoop(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.r.sweep(sessionTTL, now)
		}
	}
}
