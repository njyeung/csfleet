package fleet

import (
	"context"
	"log"
	"strings"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// watchEvents listens for Docker container die events on csfleet-* containers
// and nudges the owning worker.
func (m *Manager) watchEvents(ctx context.Context) {
	f := filters.NewArgs(
		filters.Arg("type", string(events.ContainerEventType)),
		filters.Arg("event", "die"),
	)
	msgCh, errCh := m.cli.Events(ctx, events.ListOptions{Filters: f})

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if ctx.Err() != nil {
				return
			}
			log.Printf("[fleet] docker events error: %v", err)
			return
		case msg := <-msgCh:
			name := msg.Actor.Attributes["name"]
			if !strings.HasPrefix(name, "csfleet-") {
				continue
			}
			serverName := strings.TrimPrefix(name, "csfleet-")

			m.mu.Lock()
			w := m.workers[serverName]
			m.mu.Unlock()
			if w != nil {
				w.wake()
			}
		}
	}
}
