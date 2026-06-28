package api

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// info holds the static host facts, sampled once at startup: identity and
// hardware that don't change while the orchestrator runs. The live Host field is
// filled per-request from the latest sampler snapshot.
var info orchestratorInfoResponse

// --- Static host stats

func init() {
	info.LocalIP = localIP()
	info.PublicIP = publicIP()
	info.Hostname, _ = os.Hostname()
	info.CPUModel = cpuModel()
	info.CPUCores = runtime.NumCPU()
	info.MemTotal = memTotal()
}

// cpuModel returns the CPU's marketing name from /proc/cpuinfo.
func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(key) == "model name" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// memTotal returns total physical memory in bytes from /proc/meminfo.
func memTotal() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if ok && key == "MemTotal" {
			return parseMeminfoKB(val)
		}
	}
	return 0
}

// parseMeminfoKB reads the leading kB figure of a /proc/meminfo value (e.g.
// "  16384256 kB") and returns it in bytes.
func parseMeminfoKB(val string) uint64 {
	f := strings.Fields(val)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.ParseUint(f[0], 10, 64)
	return n * 1024
}

// localIP discovers the preferred outbound interface address by opening a UDP
// socket toward a public address (no packets are actually sent).
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "<unknown>"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// publicIP asks ipinfo.io for the machine's public address.
func publicIP() string {
	resp, err := http.Get("http://ipinfo.io/ip")
	if err != nil {
		return "<unknown>"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "<unknown>"
	}
	return strings.TrimSpace(string(body))
}

// orchestratorInfo combines the static host facts with the latest live sample.
func orchestratorInfo() orchestratorInfoResponse {
	resp := info
	resp.Host = loadHostStats()
	return resp
}

func (s *Server) getOrchestratorInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, orchestratorInfo())
}

// --- Sampled Host stats ---

// hostSampleInterval is how often live host metrics are resampled and pushed to
// SSE clients. CPU utilization is a delta between consecutive reads, so this is
// also the averaging window for the reported percentages.
const hostSampleInterval = 2 * time.Second

// diskStatfsPath is the filesystem sampled for disk usage. Server installs and
// container data live under the host root, so the root volume is the meaningful
// figure for an admin watching capacity fill up.
const diskStatfsPath = "/"

// latestHost holds the most recent hostStats sample for lock-free reads by the
// info handler and the SSE snapshot. nil until the first sample lands.
var latestHost atomic.Pointer[hostStats]

// loadHostStats returns the latest sample, or a zero value before the first tick.
func loadHostStats() hostStats {
	if h := latestHost.Load(); h != nil {
		return *h
	}
	return hostStats{}
}

// sampleHostStats samples live host metrics every hostSampleInterval, stores the
// latest for the info endpoint, and calls push so connected dashboards get a live
// feed (the host SSE stream). CPU percentages come from the delta between
// consecutive /proc/stat reads, so the first tick reports zero until a baseline
// exists. Blocks until ctx is cancelled.
func sampleHostStats(ctx context.Context, push func()) {
	t := time.NewTicker(hostSampleInterval)
	defer t.Stop()

	prev := readCPUTimes()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		cur := readCPUTimes()
		h := hostStats{
			CPUPercent:     cpuDelta(prev.total, cur.total),
			PerCorePercent: perCoreDelta(prev.cores, cur.cores),
			LoadAvg:        readLoadAvg(),
			UptimeSeconds:  readUptime(),
			SampledAt:      time.Now(),
		}
		readMem(&h)
		readDisk(&h)
		prev = cur

		latestHost.Store(&h)
		push()
	}
}

// cpuLine is one /proc/stat cpu row reduced to the two figures a utilization
// delta needs: cumulative idle jiffies (idle + iowait) and the cumulative total.
type cpuLine struct {
	idle  uint64
	total uint64
}

// cpuTimes is a single /proc/stat reading: the aggregate "cpu" line plus each
// per-core "cpuN" line, in core order.
type cpuTimes struct {
	total cpuLine
	cores []cpuLine
}

// readCPUTimes parses the cpu lines of /proc/stat. They appear first in the file,
// so parsing stops at the first non-cpu line.
func readCPUTimes() cpuTimes {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}
	}
	defer f.Close()

	var ct cpuTimes
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 || !strings.HasPrefix(fields[0], "cpu") {
			break // cpu lines lead the file; nothing of interest after them
		}
		line := parseCPULine(fields[1:])
		if fields[0] == "cpu" {
			ct.total = line
		} else {
			ct.cores = append(ct.cores, line)
		}
	}
	return ct
}

// parseCPULine sums a cpu row's jiffy columns. Order is user, nice, system, idle,
// iowait, irq, softirq, steal, guest, guest_nice; idle and iowait count as idle.
func parseCPULine(vals []string) cpuLine {
	var line cpuLine
	for i, v := range vals {
		n, _ := strconv.ParseUint(v, 10, 64)
		line.total += n
		if i == 3 || i == 4 {
			line.idle += n
		}
	}
	return line
}

// cpuDelta is the busy fraction between two cumulative readings, as a percentage.
func cpuDelta(prev, cur cpuLine) float64 {
	dt := cur.total - prev.total
	if dt == 0 {
		return 0
	}
	busy := dt - (cur.idle - prev.idle)
	return float64(busy) / float64(dt) * 100
}

// perCoreDelta pairs up the per-core readings. A core missing from prev (first
// tick) reports zero.
func perCoreDelta(prev, cur []cpuLine) []float64 {
	out := make([]float64, len(cur))
	for i := range cur {
		if i < len(prev) {
			out[i] = cpuDelta(prev[i], cur[i])
		}
	}
	return out
}

// readLoadAvg reads the 1/5/15-minute load averages from /proc/loadavg.
func readLoadAvg() [3]float64 {
	var la [3]float64
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return la
	}
	f := strings.Fields(string(b))
	for i := 0; i < 3 && i < len(f); i++ {
		la[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return la
}

// readUptime reads seconds since boot from /proc/uptime.
func readUptime() uint64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(f[0], 64)
	return uint64(secs)
}

// readMem fills the memory and swap fields from /proc/meminfo. Used is derived
// from MemAvailable (the kernel's reclaim-aware estimate), not MemFree.
func readMem(h *hostStats) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var total, available, swapTotal, swapFree uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		switch key {
		case "MemTotal":
			total = parseMeminfoKB(val)
		case "MemAvailable":
			available = parseMeminfoKB(val)
		case "SwapTotal":
			swapTotal = parseMeminfoKB(val)
		case "SwapFree":
			swapFree = parseMeminfoKB(val)
		}
	}
	h.MemAvailable = available
	if total >= available {
		h.MemUsed = total - available
	}
	if swapTotal >= swapFree {
		h.SwapUsed = swapTotal - swapFree
	}
}

// readDisk fills the disk fields from statfs on diskStatfsPath.
func readDisk(h *hostStats) {
	var st unix.Statfs_t
	if err := unix.Statfs(diskStatfsPath, &st); err != nil {
		return
	}
	bs := uint64(st.Bsize)
	h.DiskTotal = st.Blocks * bs
	h.DiskUsed = (st.Blocks - st.Bfree) * bs
}
