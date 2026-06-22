//go:build integration

// Proof harness for the load-bearing property of the data path: that the real
// proxy (decide chain queues -> Go stamps a backend mark -> dnat chain does the
// kernel DNAT via @pool) produces a working *return* path. The whole reason for
// the two-chain/mark design is that the kernel's nf_nat binding — unlike a
// userspace packet rewrite — makes conntrack reverse-NAT the replies on its own.
//
// Topology is intentionally minimal: only the client lives in a network
// namespace, so its packets *ingress* a real interface and traverse the host's
// PREROUTING hooks (locally-generated traffic would not). The "backend" is an
// in-process Go UDP echo bound on the host, and the DNAT target is a host
// address — so delivery is local: no IP forwarding, no FORWARD/RPF firewall, no
// external echo tool to misbehave. The DNAT changes the destination *port*
// (27100 -> 27015); conntrack keys on the tuple, so the reverse-NAT question is
// identical to the production case where the IP changes too.
//
//	ns csf_cli 10.77.1.2  ──veth──▶  host 10.77.1.1
//	                                   ├─ decide chain (queue -> Go sets mark)
//	                                   ├─ dnat chain  (dnat to meta mark map @pool)
//	                                   └─ in-process echo on 10.77.1.1:27015
//
// It runs the question two ways:
//   - mark_dnat: the real proxy data path (installTable + the NFQUEUE handler +
//     AddBackend's @pool element). Client connects to host:27100; we require the
//     echo's reply to return on that connected socket.
//   - kernel_dnat: a plain nft `dnat` rule as an independent control, so a
//     mark_dnat failure can be pinned on our code rather than the environment.
//
// A precheck first proves the echo + local delivery work, so a no-reply result
// under mark_dnat can only mean the proxy's return path is broken.
//
// Run as root (it execs ip/nft/python3/modprobe):
//
//	go test -tags integration -c -o /tmp/proxytest ./proxy/
//	sudo /tmp/proxytest -test.run TestReturnPath -test.v
package proxy

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	nfqueue "github.com/florianl/go-nfqueue"
	"golang.org/x/sys/unix"
)

const (
	cliNS     = "csf_cli"
	hostIf    = "csf_vc"
	cliIf     = "csf_vcp"
	hostIP    = "10.77.1.1" // address the client connects to; also the DNAT target
	hostCIDR  = "10.77.1.1/24"
	cliCIDR   = "10.77.1.2/24"
	backendIP = hostIP // in-process echo binds here (local delivery, no forwarding)
	extPort   = 27100  // managed external port
)

func TestReturnPath(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root (NFQUEUE, nft nat, network namespaces)")
	}
	setup(t)

	// Precheck: prove the echo + local delivery work, independent of NAT.
	stop := startEcho(t)
	pre := probe(t, hostIP, int(backendPort))
	stop()
	t.Logf("[precheck] raw client->%s:%d reply = %q", hostIP, backendPort, pre)
	if !strings.Contains(pre, "ping") {
		diag(t)
		t.Fatalf("precheck failed: client cannot reach the echo at all; fix setup before trusting NAT results")
	}

	// The real proxy data path: decide chain queues to Go, Go stamps the backend
	// mark, the dnat chain does the kernel DNAT via @pool. The reply must return.
	t.Run("mark_dnat", func(t *testing.T) {
		p := New(Config{Table: "csfleet_test"})
		ctx, cancel := context.WithCancel(context.Background())
		if err := p.startDataPath(ctx); err != nil {
			t.Fatalf("start data path: %v", err)
		}
		t.Cleanup(func() {
			cancel()
			if p.nfq != nil {
				p.nfq.Close()
			}
			deleteTable(p.cfg.Table)
		})
		if err := p.AddBackend(extPort, backendIP); err != nil {
			t.Fatalf("add backend: %v", err)
		}

		stop := startEcho(t)
		defer stop()
		reply := probe(t, hostIP, extPort)
		t.Logf("[mark dnat] probe reply = %q", reply)
		if !strings.Contains(reply, "ping") {
			diag(t)
			t.Errorf("return path broken: client got no reply through the mark+map kernel DNAT")
		} else {
			t.Logf("CONFIRMED working: Go set meta mark, the kernel DNAT'd via @pool, conntrack reverse-NAT'd the reply")
		}
	})

	// Control: let the kernel's NAT engine do the DNAT over the same topology.
	t.Run("kernel_dnat", func(t *testing.T) {
		ruleset := fmt.Sprintf(`table ip csfleet_ctrl {
  chain prerouting {
    type nat hook prerouting priority dstnat;
    ip daddr %s udp dport %d dnat to %s:%d
  }
}
`, hostIP, extPort, backendIP, backendPort)
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(ruleset)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install control dnat: %v\n%s", err, out)
		}
		t.Cleanup(func() { try("nft", "delete", "table", "ip", "csfleet_ctrl") })

		stop := startEcho(t)
		defer stop()
		reply := probe(t, hostIP, extPort)
		t.Logf("[kernel dnat] probe reply = %q", reply)
		if strings.Contains(reply, "ping") {
			t.Logf("CONFIRMED working: kernel DNAT reverse-NATs the reply back to the connected client socket")
		} else {
			t.Errorf("control failed: kernel DNAT returned no reply — setup problem, results inconclusive")
		}
	})
}

// startDataPath is a minimal, test-only version of Proxy.Start: it installs the
// real nft table (decide + dnat chains + the @pool map) and registers the real
// NFQUEUE handler, without the DOCKER-USER rule or conntrack sysctls (irrelevant
// here — delivery is local).
func (p *Proxy) startDataPath(ctx context.Context) error {
	if err := p.installTable(); err != nil {
		return err
	}
	nfq, err := nfqueue.Open(&nfqueue.Config{
		NfQueue:      p.cfg.QueueNum,
		MaxQueueLen:  p.cfg.QueueMaxLen,
		MaxPacketLen: 0xffff,
		Copymode:     nfqueue.NfQnlCopyPacket,
		AfFamily:     unix.AF_INET,
	})
	if err != nil {
		deleteTable(p.cfg.Table)
		return err
	}
	p.nfq = nfq
	if err := nfq.RegisterWithErrorFunc(ctx, p.onPacket, p.onError); err != nil {
		nfq.Close()
		deleteTable(p.cfg.Table)
		return err
	}
	return nil
}

// startEcho runs an in-process UDP echo bound to backendIP:backendPort in the
// host namespace and returns a stop func. Replies come from that exact address,
// so a connected client only accepts them if the reverse path is correct.
func startEcho(t *testing.T) func() {
	t.Helper()
	pc, err := net.ListenPacket("udp", net.JoinHostPort(backendIP, strconv.Itoa(int(backendPort))))
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(buf[:n], from)
		}
	}()
	return func() { pc.Close() }
}

// probe sends "ping" from the client ns to ip:port over a *connected* UDP socket
// and returns whatever came back (or ""). A connected socket only accepts replies
// from that exact peer — exactly what a real CS2 client does, and what proves the
// reverse path: the reply must appear to come from ip:port, not the backend.
//
// It uses python3 rather than ncat: nmap's ncat (7.x) UDP connect mode does not
// surface the echo's reply here, so it would report a false "no reply".
func probe(t *testing.T, ip string, port int) string {
	t.Helper()
	const script = `
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(2.0)
s.connect((sys.argv[1], int(sys.argv[2])))
s.send(b"ping")
try:
    sys.stdout.write(s.recv(2048).decode("latin1"))
except Exception:
    pass
`
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "ip", "netns", "exec", cliNS,
		"python3", "-c", script, ip, strconv.Itoa(port))
	out, _ := c.CombinedOutput()
	return strings.TrimSpace(string(out))
}

func setup(t *testing.T) {
	t.Helper()
	try("modprobe", "nfnetlink_queue")
	try("modprobe", "nf_nat")
	try("modprobe", "nft_chain_nat")

	teardown() // clear any leftovers from a previous run
	t.Cleanup(teardown)

	run(t, "ip", "netns", "add", cliNS)
	run(t, "ip", "link", "add", hostIf, "type", "veth", "peer", "name", cliIf)
	run(t, "ip", "link", "set", cliIf, "netns", cliNS)
	run(t, "ip", "addr", "add", hostCIDR, "dev", hostIf)
	run(t, "ip", "link", "set", hostIf, "up")
	run(t, "ip", "netns", "exec", cliNS, "ip", "addr", "add", cliCIDR, "dev", cliIf)
	run(t, "ip", "netns", "exec", cliNS, "ip", "link", "set", cliIf, "up")
	run(t, "ip", "netns", "exec", cliNS, "ip", "link", "set", "lo", "up")
	run(t, "ip", "netns", "exec", cliNS, "ip", "route", "add", "default", "via", hostIP)

	// Local delivery doesn't need forwarding; relax rp_filter just in case.
	try("sysctl", "-q", "-w", "net.ipv4.conf.all.rp_filter=0")
	try("sysctl", "-q", "-w", "net.ipv4.conf."+hostIf+".rp_filter=0")
}

func teardown() {
	try("nft", "delete", "table", "ip", "csfleet_ctrl")
	try("nft", "delete", "table", "ip", "csfleet_test")
	try("ip", "link", "del", hostIf)
	try("ip", "netns", "del", cliNS)
}

// diag dumps host state to help distinguish a setup problem from a real result.
func diag(t *testing.T) {
	t.Helper()
	cmds := [][]string{
		{"ip", "-br", "addr"},
		{"ip", "netns", "exec", cliNS, "ip", "-br", "addr"},
		{"ip", "netns", "exec", cliNS, "ip", "route"},
		{"nft", "list", "ruleset"},
		{"sysctl", "net.ipv4.conf.all.rp_filter"},
	}
	for _, c := range cmds {
		out, _ := exec.Command(c[0], c[1:]...).CombinedOutput()
		t.Logf("$ %s\n%s", strings.Join(c, " "), strings.TrimSpace(string(out)))
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func try(name string, args ...string) { _ = exec.Command(name, args...).Run() }
