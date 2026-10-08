// Package agent runs one GoAI agent per user.
//
// The Supervisor owns the per-user prompt queue, the run lifecycle, the tool
// set and the token accounting. Every side effect (chat rows, WebSocket frames,
// preview refresh) is delegated to the Events interface, which the core package
// implements, so this package never imports it.
package agent

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/zendev-sh/goai/provider"
	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/files"
)

// Agent states reported by State and carried in the frozen `agent.state` frame.
const (
	StateIdle     = "idle"
	StateQueued   = "queued"
	StateThinking = "thinking"
	StateEditing  = "editing"
	StateError    = "error"
)

// defaultModel is used when neither the room nor the config names one.
const defaultModel = "deepseek-flash"

// fallbackModel is used when the provider rejects the room's model (Q-AGENT-13).
const fallbackModel = "deepseek-chat"

// ErrNoRunInFlight is returned by Cancel when the user has nothing to cancel.
var ErrNoRunInFlight = errors.New("agent: no run in flight")

// Events is implemented by the core package. It carries every observable side
// effect of a run: chat rows, WebSocket frames and the preview trigger.
type Events interface {
	// AgentStatus persists one kind=agent_status chat line and pushes it.
	AgentStatus(roomID string, userID int64, text string)
	// AgentState pushes the current agent.state frame plus a user.update.
	AgentState(roomID string, userID int64)
	// SiteUpdated pushes site.updated{rev} so the browser reloads the iframe.
	SiteUpdated(roomID string, userID int64)
	// RunFinished refreshes the user's preview and their admin grid row.
	RunFinished(roomID string, userID int64)
	// SystemMessage persists one canned kind=system chat line and pushes it.
	SystemMessage(roomID string, userID int64, text string)
}

// Options configures a Supervisor.
type Options struct {
	Cfg          *config.Config
	Q            *dbgen.Queries
	Files        *files.Store
	Events       Events
	Log          *zap.Logger
	NewModel     func(modelID string) provider.LanguageModel
	SystemPrompt string
}

// queuedPrompt is one accepted but not-yet-finished run.
type queuedPrompt struct {
	runID  int64
	prompt string
}

// userState is the supervisor's per-user live state. It is never persisted.
type userState struct {
	roomID string
	queue  []queuedPrompt
	// running is true while the worker goroutine owns the user; picked is
	// true once it has taken the head of the queue, so the head counts as
	// the active run and never as queue depth.
	running bool
	picked  bool
	cancel  context.CancelCauseFunc
	runID   int64
	state   string
	steps   int
	in, out int64
	// done is closed when the worker goroutine exits; it lets ResetContext
	// wait for the in-flight run to stop before wiping the history.
	done chan struct{}
}

// Supervisor queues and executes agent runs.
type Supervisor struct {
	cfg          *config.Config
	q            *dbgen.Queries
	files        *files.Store
	events       Events
	log          *zap.Logger
	newModel     func(string) provider.LanguageModel
	systemPrompt string
	// sem is the instance-wide run semaphore (Q-AGENT-17). It is acquired
	// before a run starts, so excess runs wait in their own user's queue.
	sem chan struct{}

	mu    sync.Mutex
	users map[int64]*userState
}

// New returns a Supervisor. Cfg, Q, Files and NewModel are required; a nil
// Events and a nil Log degrade to no-ops.
func New(o Options) *Supervisor {
	log := o.Log
	if log == nil {
		log = zap.NewNop()
	}
	events := o.Events
	if events == nil {
		events = nopEvents{}
	}
	prompt := o.SystemPrompt
	if prompt == "" {
		prompt = SystemPrompt
	}
	semSize := 1
	if o.Cfg != nil && o.Cfg.MaxConcurrentRuns > 0 {
		semSize = o.Cfg.MaxConcurrentRuns
	}
	return &Supervisor{
		cfg:          o.Cfg,
		q:            o.Q,
		files:        o.Files,
		events:       events,
		log:          log,
		newModel:     o.NewModel,
		systemPrompt: prompt,
		sem:          make(chan struct{}, semSize),
		users:        make(map[int64]*userState),
	}
}

// Enqueue accepts one prompt for a user.
//
// It inserts the agent_runs row with status='queued' and returns it. queued is
// true when the prompt was placed behind an already-running prompt for that
// user, in which case depth counts every waiting prompt including this one;
// when the run starts immediately queued is false and depth is 0.
func (s *Supervisor) Enqueue(ctx context.Context, roomID string, userID int64, prompt string) (runID int64, queued bool, depth int, err error) {
	run, err := s.q.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
		UserID:    userID,
		Prompt:    prompt,
		Status:    "queued",
		StartedAt: sql.NullInt64{},
	})
	if err != nil {
		return 0, false, 0, err
	}

	s.mu.Lock()
	st := s.users[userID]
	if st == nil {
		st = &userState{state: StateIdle}
		s.users[userID] = st
	}
	st.roomID = roomID
	queued = st.running
	st.queue = append(st.queue, queuedPrompt{runID: run.ID, prompt: prompt})
	if queued {
		depth = queuedDepth(st)
	} else {
		st.running = true
		st.picked = false
		st.runID = run.ID
		st.state = StateQueued
		st.done = make(chan struct{})
		go s.worker(userID)
	}
	s.mu.Unlock()

	s.emitState(userID)
	return run.ID, queued, depth, nil
}

// worker drains one user's queue, one prompt at a time, and exits when the
// queue is empty.
func (s *Supervisor) worker(userID int64) {
	for {
		// The cancellable context is created before the lock so that Cancel
		// can always interrupt a job, including one still waiting for a
		// concurrency slot.
		jobCtx, jobCancel := context.WithCancelCause(context.Background())

		s.mu.Lock()
		st := s.users[userID]
		if st == nil || len(st.queue) == 0 {
			if st != nil {
				st.running = false
				st.picked = false
				st.state = StateIdle
				st.runID = 0
				st.cancel = nil
				if st.done != nil {
					close(st.done)
					st.done = nil
				}
			}
			s.mu.Unlock()
			jobCancel(nil)
			s.emitState(userID)
			return
		}
		job := st.queue[0]
		st.queue = st.queue[1:]
		st.picked = true
		st.runID = job.runID
		st.cancel = jobCancel
		st.state = StateQueued
		st.steps, st.in, st.out = 0, 0, 0
		roomID := st.roomID
		s.mu.Unlock()

		s.emitState(userID)
		s.runPrompt(roomID, userID, job, jobCtx)
	}
}

// Cancel aborts the running prompt and clears the user's queue. It returns
// ErrNoRunInFlight when the user has nothing running.
func (s *Supervisor) Cancel(userID int64) error {
	s.mu.Lock()
	st := s.users[userID]
	if st == nil || !st.running {
		s.mu.Unlock()
		return ErrNoRunInFlight
	}
	roomID := st.roomID
	drained := st.queue
	st.queue = nil
	cancel := st.cancel
	s.mu.Unlock()

	s.markCancelled(drained)
	if cancel != nil {
		cancel(errCancelled)
	} else {
		// The job had not been picked up by the worker yet, so no run will
		// report the cancellation: do it here.
		s.events.SystemMessage(roomID, userID, msgCancelled)
	}
	return nil
}

// StopUser cancels the running prompt and drains the queue. Kick, delete,
// close, archive and reset all funnel through here.
func (s *Supervisor) StopUser(userID int64) {
	s.mu.Lock()
	st := s.users[userID]
	if st == nil {
		s.mu.Unlock()
		return
	}
	drained := st.queue
	st.queue = nil
	cancel := st.cancel
	s.mu.Unlock()

	s.markCancelled(drained)
	if cancel != nil {
		cancel(errStopped)
	}
}

// ResetContext cancels the in-flight run, drains the queue and hard-deletes the
// user's agent_messages, then records the new-session chat entry.
func (s *Supervisor) ResetContext(ctx context.Context, roomID string, userID int64) error {
	s.mu.Lock()
	st := s.users[userID]
	var drained []queuedPrompt
	var cancel context.CancelCauseFunc
	var done chan struct{}
	if st != nil {
		drained = st.queue
		st.queue = nil
		cancel = st.cancel
		done = st.done
	}
	s.mu.Unlock()

	s.markCancelled(drained)
	if cancel != nil {
		cancel(errStopped)
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err := s.q.DeleteAgentMessagesByUser(ctx, userID); err != nil {
		return err
	}
	s.events.SystemMessage(roomID, userID, msgNewSession)
	return nil
}

// StopAll aborts every run and marks it failed. It is the SIGTERM path.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	var drained []queuedPrompt
	cancels := make([]context.CancelCauseFunc, 0, len(s.users))
	for _, st := range s.users {
		drained = append(drained, st.queue...)
		st.queue = nil
		if st.cancel != nil {
			cancels = append(cancels, st.cancel)
		}
		st.runID = 0
	}
	s.mu.Unlock()

	for _, job := range drained {
		s.markFinished(job.runID, "failed", 0)
	}
	for _, cancel := range cancels {
		cancel(errShutdown)
	}
}

// State returns the user's agent state and the number of prompts waiting
// behind the running one.
func (s *Supervisor) State(userID int64) (state string, depth int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.users[userID]
	if st == nil {
		return StateIdle, 0
	}
	return st.state, queuedDepth(st)
}

// queuedDepth counts the prompts waiting behind the user's active run. The
// worker removes the head from the queue when it picks it up; before that
// first pick it still sits at the front and must not be counted as waiting.
func queuedDepth(st *userState) int {
	depth := len(st.queue)
	if st.running && !st.picked && depth > 0 {
		depth--
	}
	return depth
}

// RunID returns the id of the run the user's worker is currently on, or nil
// when nothing is in flight. The returned pointer is a fresh copy.
func (s *Supervisor) RunID(userID int64) *int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.users[userID]
	if st == nil || !st.running || st.runID == 0 {
		return nil
	}
	id := st.runID
	return &id
}

// InFlight reports whether the user has a run or a queued prompt.
func (s *Supervisor) InFlight(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.users[userID]
	return st != nil && (st.running || len(st.queue) > 0)
}

// addStepUsage folds one generation step's tokens into the user's live
// counters (Q-AGENT-2).
func (s *Supervisor) addStepUsage(userID, in, out int64) {
	s.mu.Lock()
	if st := s.users[userID]; st != nil {
		st.steps++
		st.in += in
		st.out += out
	}
	s.mu.Unlock()
}

// runUsage returns the counters of the user's current run.
func (s *Supervisor) runUsage(userID int64) (steps int, in, out int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.users[userID]; st != nil {
		return st.steps, st.in, st.out
	}
	return 0, 0, 0
}

// setState records the user's agent state.
func (s *Supervisor) setState(userID int64, state string) {
	s.mu.Lock()
	if st := s.users[userID]; st != nil {
		st.state = state
	}
	s.mu.Unlock()
}

// emitState pushes an agent.state frame for the user, when one is known.
func (s *Supervisor) emitState(userID int64) {
	s.mu.Lock()
	st := s.users[userID]
	var roomID string
	if st != nil {
		roomID = st.roomID
	}
	s.mu.Unlock()
	if roomID == "" {
		return
	}
	s.events.AgentState(roomID, userID)
}

// markCancelled finishes drained queued runs as cancelled.
func (s *Supervisor) markCancelled(jobs []queuedPrompt) {
	for _, job := range jobs {
		s.markFinished(job.runID, "cancelled", 0)
	}
}

// markFinished writes a terminal status onto one run row.
func (s *Supervisor) markFinished(runID int64, status string, steps int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := s.q.FinishAgentRun(ctx, dbgen.FinishAgentRunParams{
		Status:     status,
		FinishedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		Steps:      sql.NullInt64{Int64: int64(steps), Valid: true},
		ID:         runID,
	})
	if err != nil {
		s.log.Error("agent: finish run", zap.Int64("run", runID), zap.Error(err))
	}
}

// runTimeout is the per-run bound (AGENT_RUN_TIMEOUT_S).
func (s *Supervisor) runTimeout() time.Duration {
	if s.cfg != nil && s.cfg.AgentRunTimeout > 0 {
		return s.cfg.AgentRunTimeout
	}
	return 120 * time.Second
}

// nopEvents is used when no Events implementation is supplied.
type nopEvents struct{}

func (nopEvents) AgentStatus(string, int64, string) {}
func (nopEvents) AgentState(string, int64)          {}
func (nopEvents) SiteUpdated(string, int64)         {}
func (nopEvents) RunFinished(string, int64)         {}
func (nopEvents) SystemMessage(string, int64, string) {
}
