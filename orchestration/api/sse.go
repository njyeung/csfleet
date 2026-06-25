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
// don't reap it between fleet changes.
const keepalivePeriod = 25 * time.Second

// sseHub fans the manager's single coalescing change signal out to every
// connected SSE client. One run goroutine owns the read of that signal; each
// client gets its own buffered channel.
type sseHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[chan []byte]struct{})}
}

func (h *sseHub) add() chan []byte {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) remove(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// broadcast delivers data to every client without blocking: a client whose
// buffer is full is skipped, since the next snapshot supersedes the missed one.
func (h *sseHub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

// run owns the manager's change channel and pushes a fresh snapshot to all
// clients on every state transition until ctx is cancelled.
func (h *sseHub) run(ctx context.Context, changes <-chan struct{}, snapshot func() []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			if data := snapshot(); data != nil {
				h.broadcast(data)
			}
		}
	}
}

// handleSSE streams fleet status to one client. It sends an immediate snapshot so
// a fresh page renders without waiting for the next change, then forwards each
// broadcast until the client disconnects.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.add()
	defer s.hub.remove(ch)

	if data := s.snapshot(); data != nil {
		writeEvent(w, data)
		flusher.Flush()
	}

	keepalive := time.NewTicker(keepalivePeriod)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return // hub closed the channel (shutdown)
			}
			writeEvent(w, data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// snapshot is the SSE payload: the same merged server list GET /api/servers
// returns, marshaled once for fan-out. Returns nil on error (skips the push).
func (s *Server) snapshot() []byte {
	statuses, err := s.serverStatuses()
	if err != nil {
		log.Printf("[api] sse snapshot: %v", err)
		return nil
	}
	data, err := json.Marshal(toServerResponses(statuses))
	if err != nil {
		log.Printf("[api] sse marshal: %v", err)
		return nil
	}
	return data
}

func writeEvent(w io.Writer, data []byte) {
	fmt.Fprintf(w, "event: servers\ndata: %s\n\n", data)
}
