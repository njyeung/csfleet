package fleet

import (
	"sort"

	"csfleet/orchestrator/database"
)

// ServerStatus is the fleet's view of one server: the desired spec from the DB
// plus the actual state. actual_state is deliberately collapsed to
// "running"/"stopped" — the UI derives a transition from desired != actual, so
// startup, teardown, and crash all read as a transition without the backend
// enumerating them. last_error explains a stuck transition when present.
type ServerStatus struct {
	database.ServerRow
	ActualState phase  `json:"actual_state"`
	LastError   string `json:"last_error,omitempty"` // set while crashed/failed
}

// Status returns the fleet snapshot: one entry per worker (each worker holds
// both its last-read spec and its phase). It reads no DB and never blocks behind
// a lifecycle operation — only brief per-worker locks.
func (m *Manager) Status() []ServerStatus {
	m.mu.Lock()
	workers := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	m.mu.Unlock()

	out := make([]ServerStatus, 0, len(workers))
	for _, w := range workers {
		out = append(out, w.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
