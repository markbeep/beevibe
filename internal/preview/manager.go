package preview

import (
	"context"
	"sync"
	"time"

	"github.com/mark/beevibe/internal/config"
)

// storeCapacity bounds the preview image cache.
const storeCapacity = 512

// captureDeadline is the manager-side upper bound for one capture, including
// queueing and the renderer round trip.
const captureDeadline = 30 * time.Second

// Preview states. They are wire values (User.previewState in plans/api.md).
const (
	PreviewOK      = "ok"
	PreviewMissing = "missing"
	PreviewFailed  = "failed"
)

// job is one pending capture. gen lets a newer job or a Drop supersede it.
type job struct {
	roomID string
	userID int64
	gen    int64
}

// Manager owns the debounce timers, the single capture worker and the image
// cache. One capture runs at a time; a newer job for the same user replaces the
// queued one.
type Manager struct {
	cfg      *config.Config
	client   *Client
	onUpdate func(roomID string, userID int64)
	store    *Store

	mu       sync.Mutex
	closed   bool
	seq      int64
	queue    []*job
	queued   map[int64]int   // userID -> index into queue
	latest   map[int64]int64 // userID -> generation of the newest job or drop
	states   map[int64]string
	timerGen map[int64]int64
	timers   map[int64]*time.Timer

	wake chan struct{}
	done chan struct{}
	wg   sync.WaitGroup
}

// NewManager returns a Manager that captures through client and reports every
// state change through onUpdate (which may be nil). cfg supplies AppURL (for
// the page URL) and PreviewDebounce.
func NewManager(cfg *config.Config, client *Client, onUpdate func(roomID string, userID int64)) *Manager {
	if cfg != nil && client.AppURL == "" {
		client.AppURL = cfg.AppURL
	}
	m := &Manager{
		cfg:      cfg,
		client:   client,
		onUpdate: onUpdate,
		store:    NewStore(storeCapacity),
		queued:   make(map[int64]int),
		latest:   make(map[int64]int64),
		states:   make(map[int64]string),
		timerGen: make(map[int64]int64),
		timers:   make(map[int64]*time.Timer),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	m.wg.Add(1)
	go m.worker()
	return m
}

// Trigger schedules a capture for a user after the configured debounce. A
// second Trigger inside the window restarts the timer.
func (m *Manager) Trigger(roomID string, userID int64) {
	if m.cfg == nil {
		m.Force(roomID, userID)
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.timerGen[userID]++
	gen := m.timerGen[userID]
	if t := m.timers[userID]; t != nil {
		t.Stop()
	}
	d := m.cfg.PreviewDebounce
	m.timers[userID] = time.AfterFunc(d, func() { m.fire(roomID, userID, gen) })
	m.mu.Unlock()
}

// Force captures immediately, cancelling any pending debounce for that user.
func (m *Manager) Force(roomID string, userID int64) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.timerGen[userID]++
	if t := m.timers[userID]; t != nil {
		t.Stop()
	}
	delete(m.timers, userID)
	m.mu.Unlock()

	m.enqueue(roomID, userID)
}

// Image returns the cached preview and its revision, if any.
func (m *Manager) Image(userID int64) ([]byte, int64, bool) {
	return m.store.Get(userID)
}

// State reports the last known preview state for a user: one of PreviewOK,
// PreviewMissing or PreviewFailed.
func (m *Manager) State(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.states[userID]; ok {
		return s
	}
	return PreviewMissing
}

// Drop forgets everything about a user: cached image, state, pending debounce
// and queued job. An in-flight capture for that user is invalidated, so its
// result is discarded.
func (m *Manager) Drop(userID int64) {
	m.mu.Lock()
	m.timerGen[userID]++
	if t := m.timers[userID]; t != nil {
		t.Stop()
	}
	delete(m.timers, userID)
	delete(m.states, userID)
	// Bump the generation so any in-flight capture for this user is stale.
	m.seq++
	m.latest[userID] = m.seq

	if idx, ok := m.queued[userID]; ok {
		m.queue = append(m.queue[:idx], m.queue[idx+1:]...)
		delete(m.queued, userID)
		for i, j := range m.queue {
			m.queued[j.userID] = i
		}
	}
	m.mu.Unlock()

	m.store.Drop(userID)
}

// Close stops the worker and any pending timers. It is safe to call twice.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	for _, t := range m.timers {
		t.Stop()
	}
	m.timers = map[int64]*time.Timer{}
	m.mu.Unlock()

	close(m.done)
	m.wg.Wait()
}

// fire runs when a debounce timer elapses. It ignores stale timers.
func (m *Manager) fire(roomID string, userID int64, gen int64) {
	m.mu.Lock()
	if m.closed || m.timerGen[userID] != gen {
		m.mu.Unlock()
		return
	}
	delete(m.timers, userID)
	m.mu.Unlock()

	m.enqueue(roomID, userID)
}

// enqueue adds or replaces the pending job for a user and wakes the worker.
func (m *Manager) enqueue(roomID string, userID int64) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.seq++
	gen := m.seq
	m.latest[userID] = gen
	j := &job{roomID: roomID, userID: userID, gen: gen}

	if idx, ok := m.queued[userID]; ok {
		m.queue[idx] = j
	} else {
		m.queued[userID] = len(m.queue)
		m.queue = append(m.queue, j)
	}
	m.mu.Unlock()

	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// nextJob pops the oldest pending job, if any.
func (m *Manager) nextJob() (*job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.queue) == 0 {
		return nil, false
	}
	j := m.queue[0]
	m.queue = m.queue[1:]
	delete(m.queued, j.userID)
	for i, x := range m.queue {
		m.queued[x.userID] = i
	}
	return j, true
}

// worker captures queued jobs one at a time until Close.
func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.done:
			return
		case <-m.wake:
		}
		for {
			j, ok := m.nextJob()
			if !ok {
				break
			}
			if m.isClosed() {
				return
			}
			m.capture(j)
		}
	}
}

// isClosed reports whether Close has been called.
func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// capture runs one job: render, cache and notify. A stale job (superseded or
// dropped while it was queued or running) is discarded.
func (m *Manager) capture(j *job) {
	ctx, cancel := context.WithTimeout(context.Background(), captureDeadline)
	defer cancel()

	data, err := m.client.Capture(ctx, m.client.PageURL(j.roomID, j.userID))

	m.mu.Lock()
	stale := m.latest[j.userID] != j.gen || m.closed
	if stale {
		m.mu.Unlock()
		return
	}
	if err != nil {
		m.states[j.userID] = PreviewFailed
	} else {
		m.states[j.userID] = PreviewOK
	}
	m.mu.Unlock()

	if err != nil {
		m.notify(j.roomID, j.userID)
		return
	}
	m.store.Put(j.userID, data)
	m.notify(j.roomID, j.userID)
}

// notify reports an update to the callback, if one is registered.
func (m *Manager) notify(roomID string, userID int64) {
	if m.onUpdate != nil {
		m.onUpdate(roomID, userID)
	}
}
