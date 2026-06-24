package proxy

import (
	"fmt"
	"log"
	"net/netip"

	"golang.org/x/sys/unix"
)

// FlushConntrack deletes every conntrack entry whose reply source is this
// backend's address.
//
// IMPORTANT: Ordering is critical. See AddBackend() and RemoveBackend().
func (p *Proxy) FlushConntrack(ip string) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("flush conntrack: bad ip %q: %w", ip, err)
	}
	addr = addr.Unmap()

	flows, err := p.ct.Dump(nil)
	if err != nil {
		return fmt.Errorf("flush conntrack %s: dump: %w", ip, err)
	}

	for i := range flows {
		f := flows[i]
		if f.TupleReply.Proto.Protocol != unix.IPPROTO_UDP {
			continue
		}
		if f.TupleReply.IP.SourceAddress.Unmap() != addr {
			continue
		}
		if err := p.ct.Delete(f); err != nil {
			log.Printf("[proxy] flush conntrack %s: delete entry: %v", ip, err)
		}
	}
	return nil
}
