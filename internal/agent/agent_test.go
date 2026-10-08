package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zendev-sh/goai/provider"
	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/db"
	dbgen "github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/files"
)

// supervisorContract pins the exported surface the core package drives.
type supervisorContract interface {
	Enqueue(ctx context.Context, roomID string, userID int64, prompt string) (runID int64, queued bool, depth int, err error)
	Cancel(userID int64) error
	State(userID int64) (state string, depth int)
	RunID(userID int64) *int64
	StopUser(userID int64)
	ResetContext(ctx context.Context, roomID string, userID int64) error
	StopAll()
	InFlight(userID int64) bool
}

var _ supervisorContract = (*Supervisor)(nil)

// --- fakes ------------------------------------------------------------------

type fakeModel struct {
	id string

	mu    sync.Mutex
	calls int
	fn    func(call int, ctx context.Context, params provider.GenerateParams) (*provider.GenerateResult, error)
}

func (m *fakeModel) ModelID() string { return m.id }

func (m *fakeModel) DoGenerate(ctx context.Context, params provider.GenerateParams) (*provider.GenerateResult, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	return m.fn(call, ctx, params)
}

func (m *fakeModel) DoStream(context.Context, provider.GenerateParams) (*provider.StreamResult, error) {
	return nil, errors.New("not implemented")
}

func textResult(text string, in, out int) *provider.GenerateResult {
	return &provider.GenerateResult{
		Text:         text,
		Content:      []provider.Part{{Type: provider.PartText, Text: text}},
		Usage:        provider.Usage{InputTokens: in, OutputTokens: out},
		FinishReason: provider.FinishStop,
	}
}

func toolCallResult(name, input string, in, out int) *provider.GenerateResult {
	return &provider.GenerateResult{
		ToolCalls: []provider.ToolCall{{ID: "call-1", Name: name, Input: json.RawMessage(input)}},
		Usage:     provider.Usage{InputTokens: in, OutputTokens: out},
	}
}

// recEvents records every Events callback.
type recEvents struct {
	mu       sync.Mutex
	statuses []string
	systems  []string
	sites    int
	states   int
	finished chan struct{}
}

func newRecEvents() *recEvents {
	return &recEvents{finished: make(chan struct{}, 32)}
}

func (e *recEvents) AgentStatus(_ string, _ int64, text string) {
	e.mu.Lock()
	e.statuses = append(e.statuses, text)
	e.mu.Unlock()
}

func (e *recEvents) AgentState(string, int64) {
	e.mu.Lock()
	e.states++
	e.mu.Unlock()
}

func (e *recEvents) SiteUpdated(string, int64) {
	e.mu.Lock()
	e.sites++
	e.mu.Unlock()
}

func (e *recEvents) RunFinished(string, int64) {
	select {
	case e.finished <- struct{}{}:
	default:
	}
}

func (e *recEvents) SystemMessage(_ string, _ int64, text string) {
	e.mu.Lock()
	e.systems = append(e.systems, text)
	e.mu.Unlock()
}

func (e *recEvents) hasStatus(text string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.statuses {
		if s == text {
			return true
		}
	}
	return false
}

func (e *recEvents) hasSystem(text string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.systems {
		if s == text {
			return true
		}
	}
	return false
}

// --- harness ----------------------------------------------------------------

type harness struct {
	sup    *Supervisor
	q      *dbgen.Queries
	store  *files.Store
	events *recEvents
	roomID string
	userID int64
}

func newHarness(t *testing.T, model provider.LanguageModel, tune func(*config.Config)) *harness {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	handle, err := db.Open(ctx, filepath.Join(dir, "beevibe.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	q := dbgen.New(handle)

	store := files.New(filepath.Join(dir, "data"))
	roomID := "AB23CD"
	if _, err := q.CreateRoom(ctx, dbgen.CreateRoomParams{
		ID:        roomID,
		Model:     "deepseek-flash",
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("create room: %v", err)
	}
	user, err := q.CreateUser(ctx, dbgen.CreateUserParams{
		RoomID:    roomID,
		Name:      "Alice",
		Token:     "aaaaaaa1",
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.Seed(ctx, roomID, user.ID); err != nil {
		t.Fatalf("seed user dir: %v", err)
	}

	cfg := &config.Config{
		AgentModel:        "deepseek-flash",
		AgentMaxSteps:     20,
		AgentContextTurns: 50,
		MaxConcurrentRuns: 4,
		AgentRunTimeout:   10 * time.Second,
	}
	if tune != nil {
		tune(cfg)
	}

	events := newRecEvents()
	sup := New(Options{
		Cfg:          cfg,
		Q:            q,
		Files:        store,
		Events:       events,
		Log:          zap.NewNop(),
		NewModel:     func(string) provider.LanguageModel { return model },
		SystemPrompt: "test system prompt",
	})
	return &harness{sup: sup, q: q, store: store, events: events, roomID: roomID, userID: user.ID}
}

// awaitRun blocks until the worker reports a finished run.
func (h *harness) awaitRun(t *testing.T) {
	t.Helper()
	select {
	case <-h.events.finished:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the run to finish")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// --- tests ------------------------------------------------------------------

func TestRunExecutesWriteToolAndRecordsUsage(t *testing.T) {
	model := &fakeModel{id: "deepseek-flash"}
	model.fn = func(call int, _ context.Context, _ provider.GenerateParams) (*provider.GenerateResult, error) {
		if call == 1 {
			return toolCallResult("write_file", `{"path":"index.html","content":"<h1>Hello</h1>"}`, 100, 20), nil
		}
		return textResult("done", 120, 30), nil
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	runID, queued, depth, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "make the heading say Hello")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if queued || depth != 0 {
		t.Fatalf("queued=%v depth=%d, want false/0", queued, depth)
	}
	h.awaitRun(t)

	data, err := h.store.Read(h.roomID, h.userID, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if string(data) != "<h1>Hello</h1>" {
		t.Fatalf("index.html = %q", data)
	}

	if !h.events.hasStatus("Writing index.html…") {
		t.Fatalf("agent_status messages = %v, want Writing index.html…", h.events.statuses)
	}

	usage, err := h.q.ListUsageEventsByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(usage) != 1 {
		t.Fatalf("usage events = %d, want 1", len(usage))
	}
	if usage[0].InputTokens != 220 || usage[0].OutputTokens != 50 {
		t.Fatalf("usage row = %+v, want 220/50", usage[0])
	}

	run, err := h.q.GetAgentRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "finished" {
		t.Fatalf("run status = %q, want finished", run.Status)
	}
	if !run.Steps.Valid || run.Steps.Int64 != 2 {
		t.Fatalf("run steps = %+v, want 2", run.Steps)
	}

	user, err := h.q.GetUser(ctx, h.userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.TokensUsed != 270 {
		t.Fatalf("tokens_used = %d, want 270", user.TokensUsed)
	}
}

func TestRunWithoutToolCallsAsksAgain(t *testing.T) {
	model := &fakeModel{id: "deepseek-flash"}
	model.fn = func(int, context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
		return textResult("sure, I updated nothing", 10, 5), nil
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	before, err := h.store.List(h.roomID, h.userID)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}

	if _, _, _, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "mumble mumble"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	h.awaitRun(t)

	if !h.events.hasSystem(msgDidntCatch) {
		t.Fatalf("system messages = %v, want %q", h.events.systems, msgDidntCatch)
	}

	after, err := h.store.List(h.roomID, h.userID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("files changed without any tool call:\nbefore=%v\nafter=%v", before, after)
	}

	runs, err := h.q.ListAgentRunsByUser(ctx, h.userID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "finished" {
		t.Fatalf("runs = %+v, want one finished", runs)
	}
}

func TestCancelMidRunKeepsWrittenFiles(t *testing.T) {
	model := &fakeModel{id: "deepseek-flash"}
	var once sync.Once
	blocked := make(chan struct{})
	model.fn = func(call int, ctx context.Context, _ provider.GenerateParams) (*provider.GenerateResult, error) {
		if call == 1 {
			return toolCallResult("write_file", `{"path":"pre.html","content":"kept"}`, 10, 5), nil
		}
		once.Do(func() { close(blocked) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	runID, _, _, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "write pre.html")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("model never reached the second step")
	}

	if err := h.sup.Cancel(h.userID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	h.awaitRun(t)

	if !h.events.hasSystem(msgCancelled) {
		t.Fatalf("system messages = %v, want %q", h.events.systems, msgCancelled)
	}
	data, err := h.store.Read(h.roomID, h.userID, "pre.html")
	if err != nil {
		t.Fatalf("read pre.html: %v", err)
	}
	if string(data) != "kept" {
		t.Fatalf("pre.html = %q, want kept", data)
	}

	run, err := h.q.GetAgentRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "cancelled" {
		t.Fatalf("run status = %q, want cancelled", run.Status)
	}
}

func TestHistoryWindowStartsOnUserTurn(t *testing.T) {
	var mu sync.Mutex
	var seen []provider.Message

	model := &fakeModel{id: "deepseek-flash"}
	model.fn = func(call int, _ context.Context, params provider.GenerateParams) (*provider.GenerateResult, error) {
		if call == 1 {
			mu.Lock()
			seen = append([]provider.Message(nil), params.Messages...)
			mu.Unlock()
		}
		return textResult("ok", 1, 1), nil
	}
	h := newHarness(t, model, func(cfg *config.Config) { cfg.AgentContextTurns = 2 })
	ctx := context.Background()

	seeded := []struct{ role, text string }{
		{"assistant", "old answer"},
		{"tool", "old tool output"},
		{"user", "first request"},
		{"assistant", "second answer"},
		{"tool", "second tool output"},
		{"user", "hello again"},
	}
	for _, row := range seeded {
		parts, err := json.Marshal([]provider.Part{{Type: provider.PartText, Text: row.text}})
		if err != nil {
			t.Fatalf("marshal seed: %v", err)
		}
		seq, err := h.q.NextAgentMessageSeq(ctx, h.userID)
		if err != nil {
			t.Fatalf("next seq: %v", err)
		}
		if _, err := h.q.AppendAgentMessage(ctx, dbgen.AppendAgentMessageParams{
			UserID:    h.userID,
			Seq:       seq,
			Role:      row.role,
			Content:   string(parts),
			CreatedAt: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("append seed: %v", err)
		}
	}

	if _, _, _, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "now do it"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	h.awaitRun(t)

	mu.Lock()
	msgs := seen
	mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("model saw no messages")
	}
	if msgs[0].Role != provider.RoleUser {
		t.Fatalf("history starts with role %q, want user", msgs[0].Role)
	}
	sawUser := false
	for i, m := range msgs {
		if m.Role == provider.RoleTool && !sawUser {
			t.Fatalf("orphan tool row at index %d", i)
		}
		if m.Role == provider.RoleUser {
			sawUser = true
		}
	}
	if len(msgs[0].Content) == 0 || msgs[0].Content[0].Text != "first request" {
		t.Fatalf("first replayed message = %+v, want the first user turn", msgs[0])
	}
}

func TestPromptsQueuePerUser(t *testing.T) {
	var once sync.Once
	firstStarted := make(chan struct{})
	release := make(chan struct{})

	var mu sync.Mutex
	var prompts []string

	model := &fakeModel{id: "deepseek-flash"}
	model.fn = func(call int, ctx context.Context, params provider.GenerateParams) (*provider.GenerateResult, error) {
		last := params.Messages[len(params.Messages)-1]
		mu.Lock()
		prompts = append(prompts, last.Content[0].Text)
		mu.Unlock()
		if call == 1 {
			once.Do(func() { close(firstStarted) })
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return textResult("ok", 1, 1), nil
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	run1, queued1, depth1, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "first")
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if queued1 || depth1 != 0 {
		t.Fatalf("first: queued=%v depth=%d, want false/0", queued1, depth1)
	}
	select {
	case <-firstStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("first run never started")
	}
	if id := h.sup.RunID(h.userID); id == nil || *id != run1 {
		t.Fatalf("RunID = %v, want %d", id, run1)
	}

	run2, queued2, depth2, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "second")
	if err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
	if !queued2 || depth2 != 1 {
		t.Fatalf("second: queued=%v depth=%d, want true/1", queued2, depth2)
	}
	if state, depth := h.sup.State(h.userID); state != StateThinking || depth != 1 {
		t.Fatalf("state = %q depth = %d, want thinking/1", state, depth)
	}
	if !h.sup.InFlight(h.userID) {
		t.Fatal("InFlight = false with a queued prompt")
	}

	close(release)
	h.awaitRun(t)
	h.awaitRun(t)

	mu.Lock()
	order := append([]string(nil), prompts...)
	mu.Unlock()
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("model saw prompts %v, want [first second]", order)
	}
	waitFor(t, "RunID to clear", func() bool { return h.sup.RunID(h.userID) == nil })
	if _, depth := h.sup.State(h.userID); depth != 0 {
		t.Fatalf("final depth = %d, want 0", depth)
	}
	if runs, err := h.q.ListAgentRunsByUser(ctx, h.userID); err != nil || len(runs) != 2 {
		t.Fatalf("runs = %v (err %v), want 2", runs, err)
	} else {
		for _, run := range runs {
			if run.Status != "finished" {
				t.Fatalf("run %d status = %q, want finished", run.ID, run.Status)
			}
		}
	}
	if _, err := h.q.GetAgentRun(ctx, run2); err != nil {
		t.Fatalf("second run missing: %v", err)
	}
}

func TestResetContextDropsHistoryAndStopsRun(t *testing.T) {
	model := &fakeModel{id: "deepseek-flash"}
	var once sync.Once
	blocked := make(chan struct{})
	model.fn = func(call int, ctx context.Context, _ provider.GenerateParams) (*provider.GenerateResult, error) {
		if call == 1 {
			return toolCallResult("write_file", `{"path":"pre.html","content":"kept"}`, 10, 5), nil
		}
		once.Do(func() { close(blocked) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	runID, _, _, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "write pre.html")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("model never reached the second step")
	}

	if err := h.sup.ResetContext(ctx, h.roomID, h.userID); err != nil {
		t.Fatalf("reset context: %v", err)
	}

	if !h.events.hasSystem(msgNewSession) {
		t.Fatalf("system messages = %v, want %q", h.events.systems, msgNewSession)
	}
	if h.events.hasSystem(msgCancelled) {
		t.Fatalf("reset must not post %q, got %v", msgCancelled, h.events.systems)
	}
	rows, err := h.q.ListAgentMessages(ctx, h.userID)
	if err != nil {
		t.Fatalf("list agent messages: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("agent_messages = %d rows, want 0", len(rows))
	}
	if h.sup.InFlight(h.userID) {
		t.Fatal("InFlight after reset")
	}
	data, err := h.store.Read(h.roomID, h.userID, "pre.html")
	if err != nil || string(data) != "kept" {
		t.Fatalf("pre.html = %q (err %v), want kept", data, err)
	}
	run, err := h.q.GetAgentRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "cancelled" {
		t.Fatalf("run status = %q, want cancelled", run.Status)
	}
}

func TestRunIsMarkedRunningBeforeTheModelAnswers(t *testing.T) {
	release := make(chan struct{})
	model := &fakeModel{id: "deepseek-flash"}
	model.fn = func(int, context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
		<-release
		return textResult("ok", 1, 1), nil
	}
	h := newHarness(t, model, nil)
	ctx := context.Background()

	runID, _, _, err := h.sup.Enqueue(ctx, h.roomID, h.userID, "hello")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitFor(t, "agent_runs.status=running", func() bool {
		run, err := h.q.GetAgentRun(ctx, runID)
		return err == nil && run.Status == "running"
	})

	close(release)
	h.awaitRun(t)

	run, err := h.q.GetAgentRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "finished" {
		t.Fatalf("run status = %q, want finished", run.Status)
	}
}
