package proxy

import (
	"errors"
	"log"
	"time"

	nfqueue "github.com/florianl/go-nfqueue"
	"golang.org/x/sys/unix"
)

// onPacket is the NFQUEUE callback, invoked for the first packet of every new
// UDP flow. It parses the flow, asks the router what to do, and issues the
// verdict. The handler never rewrites the packet: for a dnat verdict it stamps
// the chosen backend's mark on the packet and accepts it, leaving the actual
// DNAT to the kernel's dnat chain (meta mark map @pool).
//
// Note: Returning a non-zero value would stop the receiver, so it always returns 0 to keep serving.
func (p *Proxy) onPacket(a nfqueue.Attribute) int {
	if a.PacketID == nil || a.Payload == nil {
		return 0
	}
	id := *a.PacketID
	pkt := *a.Payload

	client, destPort, ok := parseUDPv4(pkt)
	if !ok {
		p.nfq.SetVerdict(id, nfqueue.NfAccept)
		return 0
	}

	switch mark, v := p.r.route(client, destPort, time.Now()); v {
	case dnat:
		if err := p.nfq.SetVerdictWithMark(id, nfqueue.NfAccept, int(mark)); err != nil {
			log.Printf("[proxy] set verdict mark %d: %v", mark, err)
		}
	case drop:
		p.nfq.SetVerdict(id, nfqueue.NfDrop)
	default:
		p.nfq.SetVerdict(id, nfqueue.NfAccept)
	}
	return 0
}

// onError handles NFQUEUE receive errors. ENOBUFS means the kernel dropped
// queued packets because userspace fell behind — recoverable, so we stay quiet
// and keep reading. Returning 0 keeps the receiver alive; on shutdown the
// cancelled context breaks the loop regardless.
func (p *Proxy) onError(err error) int {
	if !errors.Is(err, unix.ENOBUFS) {
		log.Printf("[proxy] nfqueue receive: %v", err)
	}
	return 0
}
