package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// keepalivePeriod bounds how long an idle SSE connection stays silent, so proxies
// don't reap it between events.
const keepalivePeriod = 25 * time.Second

// sseClient is one connected stream. It carries two independent channels so the
// frequent host-stats feed and the rarer fleet-state feed each keep their own
// coalescing slot: a backed-up host sample never displaces a pending state event.
type sseClient struct {
	state chan []byte // fleet snapshot JSON (servers + clusters)
	host  chan []byte // orchestratorInfoResponse JSON
}

// sseHub fans events out to every connected SSE client. The state feed is
// change-driven: one run goroutine owns the signal sources (the manager's
// worker-state channel for server lifecycle, and poke for API writes that don't
// move a worker, e.g. cluster create/delete) and re-snapshots on each. The host
// feed is pushed directly by the metrics sampler on its own ticker.
type sseHub struct {
	mu      sync.Mutex
	clients map[*sseClient]struct{}
	poke    chan struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[*sseClient]struct{}), poke: make(chan struct{}, 1)}
}

// notify asks the run loop to re-snapshot and broadcast the fleet state.
// Non-blocking and coalescing: a pending signal absorbs this one, since the next
// snapshot is whole-state and supersedes any it skipped.
func (h *sseHub) notify() {
	select {
	case h.poke <- struct{}{}:
	default:
	}
}

func (h *sseHub) add() *sseClient {
	c := &sseClient{state: make(chan []byte, 1), host: make(chan []byte, 1)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *sseHub) remove(c *sseClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.state)
		close(c.host)
	}
	h.mu.Unlock()
}

// broadcastState delivers a fleet snapshot to every client without blocking: a
// client whose state buffer is full is skipped, since the next snapshot
// supersedes the missed one.
func (h *sseHub) broadcastState(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.state <- data:
		default:
		}
	}
}

// broadcastHost delivers a host-info sample to every client without blocking: a
// client whose host buffer is full is skipped, since the next sample supersedes
// the missed one.
func (h *sseHub) broadcastHost(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.host <- data:
		default:
		}
	}
}

// run owns the fleet-state signal sources and pushes a fresh snapshot to all
// clients on every state transition until ctx is cancelled.
func (h *sseHub) run(ctx context.Context, changes <-chan struct{}, snapshot func() []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
		case <-h.poke:
		}
		if data := snapshot(); data != nil {
			h.broadcastState(data)
		}
	}
}

// handleSSE streams both feeds to one client. It sends an immediate snapshot of
// each so a fresh page renders without waiting for the next change or sample,
// then forwards every broadcast until the client disconnects.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	c := s.hub.add()
	defer s.hub.remove(c)

	if data := s.fleetSnapshot(); data != nil {
		writeStateEvent(w, data)
		flusher.Flush()
	}
	if data := hostSnapshot(); data != nil {
		writeHostEvent(w, data)
		flusher.Flush()
	}

	keepalive := time.NewTicker(keepalivePeriod)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-c.state:
			if !ok {
				return // hub closed the channel (shutdown)
			}
			writeStateEvent(w, data)
			flusher.Flush()
		case data, ok := <-c.host:
			if !ok {
				return // hub closed the channel (shutdown)
			}
			writeHostEvent(w, data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// snapshotPayload is the fleet state pushed on each state event: the same data
// GET /api/servers and GET /api/clusters return. The stream is full-state
// replacement (each event supersedes the last), so servers and clusters travel
// together and a consumer always has a consistent view of both. Host info rides
// its own event (see broadcastOrchestrator).
type snapshotPayload struct {
	Servers  []serverResponse  `json:"servers"`
	Clusters []clusterResponse `json:"clusters"`
}

// snapshot marshals the current fleet state for fan-out. Returns nil on error
// (skips the push).
func (s *Server) fleetSnapshot() []byte {
	statuses, err := s.serverStatuses()
	if err != nil {
		log.Printf("[api] sse snapshot servers: %v", err)
		return nil
	}
	clusters, err := s.store.ListClusters()
	if err != nil {
		log.Printf("[api] sse snapshot clusters: %v", err)
		return nil
	}
	data, err := json.Marshal(snapshotPayload{
		Servers:  s.serverResponses(statuses),
		Clusters: toClusterResponses(clusters),
	})
	if err != nil {
		log.Printf("[api] sse marshal: %v", err)
		return nil
	}
	return data
}

// broadcastOrchestrator pushes the latest host info to every client's host feed.
// Called by the metrics sampler each tick.
func (s *Server) broadcastHost() {
	if data := hostSnapshot(); data != nil {
		s.hub.broadcastHost(data)
	}
}

// orchestratorJSON marshals the combined static facts + latest live sample.
// Returns nil on error (skips the push).
func hostSnapshot() []byte {
	data, err := json.Marshal(orchestratorInfo())
	if err != nil {
		log.Printf("[api] sse marshal orchestrator: %v", err)
		return nil
	}
	return data
}

func writeStateEvent(w io.Writer, data []byte) {
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
}

func writeHostEvent(w io.Writer, data []byte) {
	fmt.Fprintf(w, "event: host\ndata: %s\n\n", data)
}
