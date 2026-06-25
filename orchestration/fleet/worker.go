package fleet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/server"
)

// phase is a worker's internal lifecycle state. It drives the state machine and
// crash backoff; only a collapsed actual_state ("running"/"stopped") is exposed
// to callers via Status (see status.go).
type phase string

const (
	phasePending  phase = "pending"  // spawned, not yet reconciled
	phaseStarting phase = "starting" // overlay + plugins + container create in flight
	phaseRunning  phase = "running"  // container started (we treat up == ready)
	phaseStopping phase = "stopping" // teardown in flight
	phaseStopped  phase = "stopped"  // not live, by intent
	phaseCrashed  phase = "crashed"  // container died / start failed; may be in backoff
)

const (
	resyncBase  = 20 * time.Second // jittered self-read period
	backoffBase = 2 * time.Second
	backoffMax  = 2 * time.Minute
)

// worker is the autonomous controller for a single server.
type worker struct {
	name   string
	mgr    *Manager
	ctx    context.Context
	cancel context.CancelFunc
	nudge  chan struct{} // buffered(1), coalesces wake signals

	mu         sync.Mutex
	row        database.ServerRow // last spec read from the DB
	phase      phase
	inst       *server.Instance
	ip         string
	port       uint16 // external port currently registered with the proxy
	standalone bool   // had its own port (vs cluster member) at last resolve
	token      string // GSLT claimed from the pool
	startedAt  time.Time
	lastErr    string
	crashCount int
	crashedAt  time.Time
}

// wake signals the worker to reconcile. Safe from any goroutine; coalesces.
func (w *worker) wake() {
	select {
	case w.nudge <- struct{}{}:
	default:
	}
}

func (w *worker) run() {
	defer w.mgr.wg.Done()
	defer w.mgr.workerExited(w.name, w)

	w.reconcile()

	ticker := time.NewTicker(jitter())
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.bringDown()
			return
		case <-w.nudge:
			w.reconcile()
		case <-ticker.C:
			w.reconcile()
			ticker.Reset(jitter()) // re-jitter so workers don't realign
		}
	}
}

// reconcile reads the DB and performs at most one transition toward desired state.
func (w *worker) reconcile() {
	// ResolveServer collapses the server row and its cluster into the effective
	// spec: external port, auto_token, restart/stop and accepting all post-inheritance.
	eff, err := w.mgr.store.ResolveServer(w.name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.reap()
			return
		}
		log.Printf("[fleet/%s] resolve spec: %v", w.name, err)
		return
	}
	row := eff.Row

	w.mu.Lock()
	w.row = row
	w.standalone = eff.Standalone
	ph, curPort := w.phase, w.port
	w.mu.Unlock()

	// crash check
	if ph == phaseRunning && !w.containerAlive() {
		w.handleCrash()
		ph = phaseCrashed
	}

	if row.DesiredState == "running" {
		switch ph {
		case phaseRunning:

			// port changed
			if curPort != eff.Port {
				w.rebind(eff.Port)
			}

			w.checkLifecycle(eff)

		case phaseStopped, phaseCrashed, phasePending:
			if ph == phaseCrashed && w.backingOff() {
				return // a timer will wake us when the backoff elapses
			}
			w.bringUp(eff)
		}
		return
	}

	// desired == stopped
	if ph == phaseRunning {
		w.bringDown()
	}
}

// reap tears the server down and exits the worker; called when its row is gone.
func (w *worker) reap() {
	log.Printf("[fleet/%s] row deleted, reaping", w.name)
	w.bringDown()

	w.mu.Lock()
	standalone, port := w.standalone, w.port
	w.mu.Unlock()
	if standalone && port != 0 {
		w.mgr.proxy.Unmanage(port) // stop dropping packets for a deleted standalone port
	}
	w.cancel() // run loop exits via ctx.Done
}

// bringUp enforces the safety ordering Flush -> Start -> Add. On any failure it
// rolls back and marks the worker crashed (so it retries with backoff).
func (w *worker) bringUp(eff database.EffectiveServer) {
	row, extPort := eff.Row, eff.Port

	w.setPhase(phaseStarting)
	w.mgr.signalChange()

	if err := w.mgr.proxy.FlushConntrack(row.IP); err != nil {
		w.fail("flush conntrack", err)
		return
	}

	cluster := ""
	if row.Cluster != nil {
		cluster = *row.Cluster
	}

	env, err := w.mgr.store.LoadEnv(row.Name, cluster)
	if err != nil {
		w.fail("load env", err)
		return
	}

	// An explicit SRCDS_TOKEN in the resolved env always wins; otherwise claim a
	// free token from the pool when auto_token is set.
	claimed := ""
	if env["SRCDS_TOKEN"] == "" && eff.AutoToken {
		if claimed = w.mgr.claimToken(); claimed != "" {
			env["SRCDS_TOKEN"] = claimed
		} else {
			log.Printf("[fleet/%s] no GSLT free in pool, starting tokenless", row.Name)
		}
	}

	pluginNames, err := w.mgr.store.EffectivePlugins(row.Name, cluster)
	if err != nil {
		w.mgr.releaseToken(claimed)
		w.fail("load plugin list", err)
		return
	}
	configNames, err := w.mgr.store.EffectiveConfigs(row.Name, cluster)
	if err != nil {
		w.mgr.releaseToken(claimed)
		w.fail("load config list", err)
		return
	}
	configs := make([]server.ConfigPayload, 0, len(configNames))
	for _, name := range configNames {
		cf, err := w.mgr.store.GetConfigFile(name)
		if err != nil {
			w.mgr.releaseToken(claimed)
			w.fail(fmt.Sprintf("load config %q", name), err)
			return
		}
		configs = append(configs, server.ConfigPayload{Name: cf.Name, Content: cf.Content})
	}

	def := server.Definition{Name: row.Name, Network: "csfleet", IP: row.IP, Env: env}
	inst, err := server.Start(w.ctx, w.mgr.cli, w.mgr.root, def, pluginNames, configs, w.mgr.store.LoadManifest)
	if err != nil {
		w.mgr.releaseToken(claimed)
		w.fail("server start", err)
		return
	}

	if err := w.mgr.proxy.AddBackend(extPort, row.IP); err != nil {
		inst.Stop(context.Background(), w.mgr.cli)
		w.mgr.releaseToken(claimed)
		w.fail("add backend", err)
		return
	}

	w.mu.Lock()
	w.inst = inst
	w.ip = row.IP
	w.port = extPort
	w.token = claimed
	w.startedAt = time.Now()
	w.phase = phaseRunning
	w.lastErr = ""
	w.crashCount = 0
	w.mu.Unlock()
	w.mgr.signalChange()

	log.Printf("[fleet/%s] up on port %d -> %s", row.Name, extPort, row.IP)
}

// bringDown enforces Remove -> Flush -> Stop. Idempotent: no-op when not live.
func (w *worker) bringDown() {
	w.mu.Lock()
	if w.phase != phaseRunning && w.phase != phaseStopping {
		w.mu.Unlock()
		return
	}
	w.phase = phaseStopping
	w.mu.Unlock()
	w.mgr.signalChange()

	w.teardownLive("stop")

	w.setPhase(phaseStopped)
	w.mgr.signalChange()
}

// teardownLive removes the backend, flushes conntrack, stops the container, and
// releases the GSLT. Shared by bringDown and handleCrash. Safe to call twice.
func (w *worker) teardownLive(reason string) {
	w.mu.Lock()
	inst, port, ip, token := w.inst, w.port, w.ip, w.token
	w.inst, w.token = nil, ""
	w.mu.Unlock()

	if inst == nil {
		return
	}

	// when we stop a server, we want to keep its port binded
	// so that the user can start it back up and maintain the same port.
	if err := w.mgr.proxy.RemoveBackendDoNotUnbind(port, ip); err != nil {
		log.Printf("[fleet/%s] remove backend (%s): %v", w.name, reason, err)
	}
	if err := w.mgr.proxy.FlushConntrack(ip); err != nil {
		log.Printf("[fleet/%s] flush conntrack (%s): %v", w.name, reason, err)
	}
	if err := inst.Stop(context.Background(), w.mgr.cli); err != nil {
		log.Printf("[fleet/%s] stop (%s): %v", w.name, reason, err)
	}
	w.mgr.releaseToken(token)
}

// handleCrash cleans up after an unexpected container exit and arms a backoff retry.
func (w *worker) handleCrash() {
	log.Printf("[fleet/%s] container exited unexpectedly", w.name)
	w.teardownLive("crash")
	w.markCrashed("container exited")
}

// fail rolls a failed bringUp into the crashed/backoff path.
func (w *worker) fail(op string, err error) {
	log.Printf("[fleet/%s] %s: %v", w.name, op, err)
	w.markCrashed(op + ": " + err.Error())
}

func (w *worker) markCrashed(reason string) {
	w.mu.Lock()
	w.phase = phaseCrashed
	w.lastErr = reason
	w.crashCount++
	w.crashedAt = time.Now()
	n := w.crashCount
	w.mu.Unlock()
	w.mgr.signalChange()
	time.AfterFunc(backoff(n), w.wake)
}

// rebind moves a live backend to a new external port (cluster/own port changed).
// When this drains the old port (the last cluster member moved off, or a
// standalone's sole backend left) it unmanages the old port so it stops
// black-holing packets rather than lingering as an empty managed pool.
func (w *worker) rebind(newPort uint16) {
	w.mu.Lock()
	oldPort, ip := w.port, w.ip
	w.mu.Unlock()

	if err := w.mgr.proxy.RemoveBackendMaybeUnbind(oldPort, ip); err != nil {
		log.Printf("[fleet/%s] rebind remove backend: %v", w.name, err)
	}
	if err := w.mgr.proxy.AddBackend(newPort, ip); err != nil {
		w.fail("rebind add backend", err)
		return
	}
	w.mu.Lock()
	w.port = newPort
	w.mu.Unlock()
	w.mgr.signalChange()
	log.Printf("[fleet/%s] rebound %s to port %d", w.name, ip, newPort)
}

// checkLifecycle applies the resolved restart/stop hours to a running server.
// Both use the <= 0 == "no limit" convention from ResolveServer.
func (w *worker) checkLifecycle(eff database.EffectiveServer) {
	w.mu.Lock()
	age := time.Since(w.startedAt)
	w.mu.Unlock()

	// Stop a server:
	// - Set desired state to stop
	// - Bring down server
	if eff.StopHrs > 0 && age >= hrs(eff.StopHrs) {
		log.Printf("[fleet/%s] hit stop_after_hrs (%.2fh), stopping", w.name, eff.StopHrs)
		if err := w.mgr.store.UpdateServerDesiredState(w.name, "stopped"); err != nil {
			log.Printf("[fleet/%s] persist stop: %v", w.name, err)
			return
		}
		w.bringDown()
		return
	}

	// Restart a server:
	// - Bring down a server
	// - Let reconcile loop restart it eventually
	if eff.RestartHrs > 0 && age >= hrs(eff.RestartHrs) {
		log.Printf("[fleet/%s] hit restart_after_hrs (%.2fh), restarting", w.name, eff.RestartHrs)
		w.bringDown()
		// next reconcile brings it back up with a fresh clock
		w.wake()
	}
}

func (w *worker) containerAlive() bool {
	w.mu.Lock()
	inst := w.inst
	w.mu.Unlock()
	if inst == nil {
		return false
	}
	info, err := w.mgr.cli.ContainerInspect(w.ctx, inst.ContainerID)
	if err != nil {
		return false // gone
	}
	return info.State != nil && info.State.Running
}

func (w *worker) backingOff() bool {
	w.mu.Lock()
	since, n := time.Since(w.crashedAt), w.crashCount
	w.mu.Unlock()
	return since < backoff(n)
}

func (w *worker) setPhase(p phase) {
	w.mu.Lock()
	w.phase = p
	w.mu.Unlock()
}

func (w *worker) snapshot() ServerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ServerStatus{
		ServerRow:   w.row,
		ActualState: string(w.phase),
		LastError:   w.lastErr,
	}
}

func hrs(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

func jitter() time.Duration {
	half := resyncBase / 2
	return half + time.Duration(rand.Int63n(int64(resyncBase)))
}

func backoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	if n > 30 {
		return backoffMax
	}
	d := backoffBase << (n - 1)
	if d > backoffMax || d <= 0 {
		return backoffMax
	}
	return d
}
