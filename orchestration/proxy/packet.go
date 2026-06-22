package proxy

import (
	"encoding/binary"
	"net/netip"

	"golang.org/x/sys/unix"
)

// parseUDPv4 extracts the source addr:port and the destination port
// from an IPv4/UDP packet. ok is false for anything that isn't well-formed
// IPv4/UDP, which the caller accepts unmodified.
func parseUDPv4(pkt []byte) (client netip.AddrPort, destPort uint16, ok bool) {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return netip.AddrPort{}, 0, false
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl+8 || pkt[9] != unix.IPPROTO_UDP {
		return netip.AddrPort{}, 0, false
	}
	src := netip.AddrFrom4([4]byte{pkt[12], pkt[13], pkt[14], pkt[15]})
	srcPort := binary.BigEndian.Uint16(pkt[ihl : ihl+2])
	destPort = binary.BigEndian.Uint16(pkt[ihl+2 : ihl+4])
	return netip.AddrPortFrom(src, srcPort), destPort, true
}
