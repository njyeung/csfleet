package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// installDockerUser authorizes the forwarded game traffic the proxy DNATs into
// the bridge. A DNAT'd client packet becomes forwarded (host -> bridge), so it
// traverses docker's FORWARD chain, which defaults to DROP for anything not
// published with -p. The rule lives in DOCKER-USER.
//
//	iptables -I DOCKER-USER -p udp -d <subnet> --dport <port> -j ACCEPT
func installDockerUser(subnet string, port uint16) error {
	return dockerUserRule("-I", subnet, port)
}

func removeDockerUser(subnet string, port uint16) error {
	return dockerUserRule("-D", subnet, port)
}

func dockerUserRule(op, subnet string, port uint16) error {
	cmd := exec.Command("iptables", op, "DOCKER-USER",
		"-p", "udp", "-d", subnet, "--dport", strconv.Itoa(int(port)), "-j", "ACCEPT")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables %s DOCKER-USER: %w: %s", op, err, out)
	}
	return nil
}

// setConntrackUDPTimeouts pins the UDP conntrack timeouts low. Game flows are
// steady, so a short timeout means a silently-gone client's conntrack entry
// expires quickly and its next packet returns to the handler as ct state new
// rather than being NAT'd in-kernel to a backend that may have changed.
func setConntrackUDPTimeouts(seconds int) error {
	v := strconv.Itoa(seconds)
	for _, key := range []string{
		"/proc/sys/net/netfilter/nf_conntrack_udp_timeout",
		"/proc/sys/net/netfilter/nf_conntrack_udp_timeout_stream",
	} {
		if err := os.WriteFile(key, []byte(v), 0o644); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}
