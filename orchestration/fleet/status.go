package fleet

import (
	"sort"

	"csfleet/orchestrator/database"
)

// ServerStatus is the fleet's view of one server: the desired spec from the DB
// plus the actual state. actual_state is the worker's full lifecycle phase
// (pending/starting/running/stopping/stopped/crashed) so the UI can show exactly
// where a transition is; it reads "stopped" for a server that has no worker yet.
// last_error explains a stuck transition when present.
type ServerStatus struct {
	database.ServerRow
	ActualState string
	LastError   string // set while crashed/failed
}

// Status returns the fleet snapshot: one entry per worker (each worker holds
// both its last-read spec and its phase).
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
