package fleet

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/docker/docker/client"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/proxy"
)

const discoveryInterval = 30 * time.Second

// Manager is the dispatcher. It owns no lifecycle logic itself: a periodic
// discovery pass spawns one worker per server row, and each worker reconciles
// its own server against the database (the source of truth). The Manager only
// provides shared services the workers need: a GSLT allocator and a status
// change signal.
type Manager struct {
	store *database.Store
	proxy *proxy.Proxy
	cli   *client.Client
	root  string

	mu      sync.Mutex
	workers map[string]*worker

	tokensMu sync.Mutex
	inUse    map[string]struct{}

	changeCh chan struct{} // buffered(1), coalesces status-change notifications
	nudge    chan struct{} // buffered(1), wakes the discovery loop

	wg     sync.WaitGroup // tracks live worker goroutines
	cancel context.CancelFunc
}

func New(store *database.Store, px *proxy.Proxy, cli *client.Client, root string) *Manager {
	return &Manager{
		store:    store,
		proxy:    px,
		cli:      cli,
		root:     root,
		workers:  make(map[string]*worker),
		inUse:    make(map[string]struct{}),
		changeCh: make(chan struct{}, 1),
		nudge:    make(chan struct{}, 1),
	}
}

// Nudge wakes the discovery loop: it spawns workers for any new rows and signals
// every worker to re-read the DB. API handlers call this after writing intent.
func (m *Manager) Nudge() {
	select {
	case m.nudge <- struct{}{}:
	default:
	}
}

// Changes returns a coalescing channel that fires whenever any worker changes
// state. A future SSE hub reads it and re-snapshots Status(). Buffered(1), so a
// missed read just means the next snapshot already reflects the change.
func (m *Manager) Changes() <-chan struct{} { return m.changeCh }

// Run is the discovery loop. It blocks until ctx is cancelled, then waits for
// every worker to tear down.
func (m *Manager) Run(ctx context.Context) error {
	ctx, m.cancel = context.WithCancel(ctx)

	go m.watchEvents(ctx)

	m.dispatch(ctx)

	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.wg.Wait() // workers cancel via ctx and tear themselves down
			return nil
		case <-m.nudge:
			m.dispatch(ctx)
		case <-ticker.C:
			m.dispatch(ctx)
		}
	}
}

// Stop signals the Run loop (and all workers) to exit.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// dispatch is the discovery pass: spawn a worker for any server lacking one, then
// wake every worker so it re-reads its row.
func (m *Manager) dispatch(ctx context.Context) {
	rows, err := m.store.ListServers()
	if err != nil {
		log.Printf("[fleet] list servers: %v", err)
	}

	m.mu.Lock()
	for _, row := range rows {
		if _, ok := m.workers[row.Name]; !ok {
			m.spawn(ctx, row.Name)
		}
	}
	workers := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	m.mu.Unlock()

	for _, w := range workers {
		w.wake()
	}
}

// spawnLocked starts a worker goroutine for name. Caller holds m.mu.
func (m *Manager) spawn(ctx context.Context, name string) {
	wctx, cancel := context.WithCancel(ctx)
	w := &worker{
		name:   name,
		mgr:    m,
		ctx:    wctx,
		cancel: cancel,
		nudge:  make(chan struct{}, 1),
		row:    database.ServerRow{Name: name},
		phase:  phasePending,
	}
	m.workers[name] = w
	m.wg.Add(1)
	go w.run()
	log.Printf("[fleet] worker spawned for %s", name)
}

// workerExited removes a worker from the map once its goroutine returns. The
// identity check avoids deleting a freshly respawned worker of the same name.
func (m *Manager) workerExited(name string, self *worker) {
	m.mu.Lock()
	if m.workers[name] == self {
		delete(m.workers, name)
	}
	m.mu.Unlock()
	m.signalChange()
}

// signalChange notifies the change channel without blocking.
func (m *Manager) signalChange() {
	select {
	case m.changeCh <- struct{}{}:
	default:
	}
}

// claimToken hands out a GSLT from the pool that no live server currently holds,
// or "" if none are free. The allocator is the single source of truth for which
// tokens are in use, so concurrent workers never double-claim.
func (m *Manager) claimToken() string {
	m.tokensMu.Lock()
	defer m.tokensMu.Unlock()

	tokens, err := m.store.ListGSLTTokens()
	if err != nil {
		log.Printf("[fleet] list gslt tokens: %v", err)
		return ""
	}
	for _, t := range tokens {
		if _, used := m.inUse[t]; !used {
			m.inUse[t] = struct{}{}
			return t
		}
	}
	return ""
}

func (m *Manager) releaseToken(token string) {
	if token == "" {
		return
	}
	m.tokensMu.Lock()
	delete(m.inUse, token)
	m.tokensMu.Unlock()
}
