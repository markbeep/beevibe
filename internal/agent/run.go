package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/db/gen"
)

// Canned system chat lines this package is responsible for (verbatim).
const (
	msgDidntCatch = "I didn't catch that — please say it again."
	msgCancelled  = "Prompt cancelled."
	msgTimeout    = "The agent timed out — please try again."
	msgError      = "Something went wrong — please try again."
	msgNewSession = "New agent session — the agent forgot the earlier conversation."
)

// Cancellation causes. The cause decides how a run is finalized: a user or
// admin cancel is reported in chat, a kick/delete/deploy-shutdown is not.
var (
	errCancelled = errors.New("agent: run cancelled")
	errStopped   = errors.New("agent: user stopped")
	errShutdown  = errors.New("agent: shutdown")
)

// runState carries one run's GoAI hooks. The token counters live on the
// user's supervisor state; only the tool-call bookkeeping is run-scoped.
type runState struct {
	s      *Supervisor
	roomID string
	userID int64

	mu        sync.Mutex
	toolCalls int
	editing   bool
}

// stepDone accumulates one generation step's tokens (Q-AGENT-2).
func (r *runState) stepDone(step goai.StepResult) {
	r.s.addStepUsage(r.userID, int64(step.Usage.InputTokens), int64(step.Usage.OutputTokens))
}

// toolStart emits the per-tool status line and flips the state to editing on
// the first tool call of the run (Q-AGENT-6, Q-AGENT-10c).
func (r *runState) toolStart(info goai.ToolCallStartInfo) {
	r.mu.Lock()
	r.toolCalls++
	first := !r.editing
	r.editing = true
	r.mu.Unlock()

	if line, ok := statusLine(info.ToolName, info.Input); ok {
		r.s.events.AgentStatus(r.roomID, r.userID, line)
	}
	if first {
		r.s.setState(r.userID, StateEditing)
		r.s.emitState(r.userID)
	}
}

// toolEnd reloads the user's iframe after every successful tool call (D9).
func (r *runState) toolEnd(info goai.ToolCallInfo) {
	if info.Error == nil {
		r.s.events.SiteUpdated(r.roomID, r.userID)
	}
}

func (r *runState) toolCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolCalls
}

// runPrompt executes one queued prompt and finalizes its agent_runs row.
func (s *Supervisor) runPrompt(roomID string, userID int64, job queuedPrompt, base context.Context) {
	// One global concurrency slot per run (Q-AGENT-17). Excess runs wait here
	// while their user's state stays "queued".
	select {
	case s.sem <- struct{}{}:
	case <-base.Done():
		s.abortQueued(roomID, userID, job.runID, base)
		return
	}
	defer func() { <-s.sem }()

	ctx, cancel := context.WithTimeout(base, s.runTimeout())
	defer cancel()

	if err := s.q.StartAgentRun(ctx, dbgen.StartAgentRunParams{
		StartedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:        job.runID,
	}); err != nil {
		s.log.Error("agent: mark run running", zap.Int64("run", job.runID), zap.Error(err))
	}
	s.setState(userID, StateThinking)
	s.emitState(userID)

	run := &runState{s: s, roomID: roomID, userID: userID}

	// History is read before the new user turn is appended, so the replay
	// window never contains the current prompt twice.
	history := s.loadHistory(ctx, userID)
	seq, err := s.appendTurn(ctx, userID, provider.RoleUser, promptParts(job.prompt))
	if err != nil {
		s.log.Error("agent: persist user turn", zap.Int64("run", job.runID), zap.Error(err))
	}
	messages := append(history, provider.Message{
		Role:    provider.RoleUser,
		Content: promptParts(job.prompt),
	})

	result, err := s.callModel(ctx, run, roomID, userID, messages)
	steps, inTok, outTok := s.runUsage(userID)

	fctx, fcancel := finalizeCtx(ctx)
	defer fcancel()

	if err == nil {
		s.markFinished(job.runID, "finished", steps)
		if len(result.ResponseMessages) > 0 {
			s.appendResponseMessages(fctx, userID, seq, result.ResponseMessages)
		}
		// A run that touched no tool understood nothing (Q-AGENT-10b).
		if run.toolCallCount() == 0 {
			s.events.SystemMessage(roomID, userID, msgDidntCatch)
		}
		s.recordUsage(fctx, job.runID, userID, inTok, outTok)
	} else {
		status, text, logError := runOutcome(ctx, err)
		if logError {
			s.log.Error("agent: run failed", zap.Int64("run", job.runID), zap.Error(err))
		}
		s.markFinished(job.runID, status, steps)
		if text != "" {
			s.events.SystemMessage(roomID, userID, text)
		}
		// Files already written are kept (Q-AGENT-15); tokens already burnt
		// are recorded.
		s.recordUsage(fctx, job.runID, userID, inTok, outTok)
	}

	s.setState(userID, StateIdle)
	s.emitState(userID)
	s.events.RunFinished(roomID, userID)
}

// abortQueued finalizes a run that was cancelled before it ever got a slot.
func (s *Supervisor) abortQueued(roomID string, userID int64, runID int64, base context.Context) {
	status := "cancelled"
	if errors.Is(context.Cause(base), errShutdown) {
		status = "failed"
	}
	s.markFinished(runID, status, 0)
	s.setState(userID, StateIdle)
	s.emitState(userID)
	s.events.RunFinished(roomID, userID)
}

// runOutcome classifies a failed run: the terminal status, the canned chat
// line (empty when the user should not be told) and whether to log at ERROR.
func runOutcome(ctx context.Context, err error) (status, text string, logError bool) {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "failed", msgTimeout, false
	case errors.Is(cause, errShutdown):
		return "failed", "", false
	case errors.Is(cause, errStopped):
		return "cancelled", "", false
	case errors.Is(cause, errCancelled), errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return "cancelled", msgCancelled, false
	default:
		return "failed", msgError, true
	}
}

// finalizeCtx returns a fresh context for the terminal DB writes: the run's own
// context is already cancelled on every non-success path.
func finalizeCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
}

// callModel runs the GoAI tool loop, retrying once with the fallback model when
// the provider rejects the room's model (Q-AGENT-13).
func (s *Supervisor) callModel(ctx context.Context, run *runState, roomID string, userID int64, messages []provider.Message) (*goai.TextResult, error) {
	if s.newModel == nil {
		return nil, errors.New("agent: no model factory configured")
	}
	modelID := s.roomModel(ctx, roomID)
	maxSteps := 1
	if s.cfg != nil && s.cfg.AgentMaxSteps > 0 {
		maxSteps = s.cfg.AgentMaxSteps
	}
	tools := buildTools(s.q, s.files, roomID, userID)
	system := s.systemText(roomID, userID)

	generate := func(id string) (*goai.TextResult, error) {
		model := s.newModel(id)
		if model == nil {
			return nil, errors.New("agent: model factory returned nil")
		}
		// The current prompt is already the last element of messages (see
		// runPrompt): GoAI's WithPrompt would prepend it ahead of the replayed
		// history, replaying the conversation out of order.
		//
		// WithSequentialToolExecution keeps status lines and file writes in
		// model order.
		return goai.GenerateText(ctx, model,
			goai.WithSystem(system),
			goai.WithMessages(messages...),
			goai.WithTools(tools...),
			goai.WithMaxSteps(maxSteps),
			goai.WithSequentialToolExecution(),
			goai.WithOnStepFinish(run.stepDone),
			goai.WithOnToolCallStart(run.toolStart),
			goai.WithOnToolCall(run.toolEnd),
		)
	}

	result, err := generate(modelID)
	if err == nil {
		return result, nil
	}
	var apiErr *goai.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 400 || apiErr.StatusCode == 404) && modelID != fallbackModel {
		s.log.Warn("agent: model rejected by provider, falling back to deepseek-chat",
			zap.String("model", modelID), zap.Int("status", apiErr.StatusCode))
		return generate(fallbackModel)
	}
	return nil, err
}

// roomModel is the model configured on the room, defaulting to the config.
func (s *Supervisor) roomModel(ctx context.Context, roomID string) string {
	if s.q != nil {
		if room, err := s.q.GetRoom(ctx, roomID); err == nil && room.Model != "" {
			return room.Model
		}
	}
	if s.cfg != nil && s.cfg.AgentModel != "" {
		return s.cfg.AgentModel
	}
	return defaultModel
}

// loadHistory replays the last AGENT_CONTEXT_TURNS turns, dropping leading
// rows until the window starts on a user turn (D11) so the model never sees an
// orphan tool message.
func (s *Supervisor) loadHistory(ctx context.Context, userID int64) []provider.Message {
	turns := 50
	if s.cfg != nil && s.cfg.AgentContextTurns > 0 {
		turns = s.cfg.AgentContextTurns
	}
	rows, err := s.q.ListAgentMessagesWindow(ctx, dbgen.ListAgentMessagesWindowParams{
		UserID: userID,
		Limit:  int64(3 * turns),
	})
	if err != nil {
		s.log.Error("agent: load history", zap.Error(err))
		return nil
	}
	start := 0
	for start < len(rows) && rows[start].Role != string(provider.RoleUser) {
		start++
	}
	rows = rows[start:]

	history := make([]provider.Message, 0, len(rows))
	for _, row := range rows {
		var parts []provider.Part
		if err := json.Unmarshal([]byte(row.Content), &parts); err != nil || len(parts) == 0 {
			parts = promptParts(row.Content)
		}
		history = append(history, provider.Message{
			Role:    provider.Role(row.Role),
			Content: parts,
		})
	}
	return history
}

// appendTurn persists one agent_messages row and returns the seq it used.
func (s *Supervisor) appendTurn(ctx context.Context, userID int64, role provider.Role, parts []provider.Part) (int64, error) {
	data, err := json.Marshal(parts)
	if err != nil {
		return 0, err
	}
	seq, err := s.q.NextAgentMessageSeq(ctx, userID)
	if err != nil {
		return 0, err
	}
	if _, err := s.q.AppendAgentMessage(ctx, dbgen.AppendAgentMessageParams{
		UserID:    userID,
		Seq:       seq,
		Role:      string(role),
		Content:   string(data),
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return seq, err
	}
	return seq, nil
}

// appendResponseMessages persists the assistant and tool turns of a finished
// run, continuing the seq from the user turn (D12).
func (s *Supervisor) appendResponseMessages(ctx context.Context, userID, seq int64, msgs []provider.Message) {
	for _, m := range msgs {
		data, err := json.Marshal(m.Content)
		if err != nil {
			s.log.Error("agent: marshal response message", zap.Error(err))
			continue
		}
		seq++
		if _, err := s.q.AppendAgentMessage(ctx, dbgen.AppendAgentMessageParams{
			UserID:    userID,
			Seq:       seq,
			Role:      string(m.Role),
			Content:   string(data),
			CreatedAt: time.Now().Unix(),
		}); err != nil {
			s.log.Error("agent: append response message", zap.Error(err))
			return
		}
	}
}

// recordUsage writes one usage_events row and adds the tokens to the user's
// running total (Q-AGENT-2). It is a no-op when nothing was consumed.
func (s *Supervisor) recordUsage(ctx context.Context, runID, userID, in, out int64) {
	if in+out <= 0 {
		return
	}
	if _, err := s.q.InsertUsageEvent(ctx, dbgen.InsertUsageEventParams{
		RunID:        runID,
		UserID:       userID,
		InputTokens:  in,
		OutputTokens: out,
		CreatedAt:    time.Now().Unix(),
	}); err != nil {
		s.log.Error("agent: insert usage event", zap.Error(err))
	}
	if err := s.q.AddToUserTokensUsed(ctx, dbgen.AddToUserTokensUsedParams{
		TokensUsed: in + out,
		ID:         userID,
	}); err != nil {
		s.log.Error("agent: add to user tokens used", zap.Error(err))
	}
}

// systemText is the global system prompt plus the current file listing, so
// every run's context carries the site contents (Q-AGENT-12).
func (s *Supervisor) systemText(roomID string, userID int64) string {
	base := s.systemPrompt
	if s.files == nil {
		return base
	}
	listing, err := siteListing(s.files, roomID, userID)
	if err != nil || listing == "" {
		return base
	}
	return base + "\n\nCurrent files in the website directory:\n" + listing
}

// statusLine renders the frozen agent_status text for a tool call. The second
// result is false for tool names that carry no status line.
func statusLine(tool string, input json.RawMessage) (string, bool) {
	switch tool {
	case toolListFiles:
		return "Listing files…", true
	case toolReadFile:
		return withPath("Reading", input), true
	case toolWriteFile:
		return withPath("Writing", input), true
	case toolDeleteFile:
		return withPath("Deleting", input), true
	}
	return "", false
}

func withPath(verb string, input json.RawMessage) string {
	p := sanitizePath(toolPath(input))
	if p == "" {
		return verb + "…"
	}
	return verb + " " + p + "…"
}

// toolPath extracts the model-supplied "path" argument, if any.
func toolPath(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	return in.Path
}

// sanitizePath reduces a model-supplied path to relative-path characters:
// traversal segments and any leading slash are dropped and the result is
// truncated to 120 bytes, so no server path can ever reach a chat line (T6).
func sanitizePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return ""
	}
	segments := make([]string, 0, 4)
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".", "..":
			continue
		}
		segments = append(segments, seg)
	}
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '/', r == '.', r == '-', r == '_', r == ' ', r == '+', r == '@':
			return r
		}
		return -1
	}, strings.Join(segments, "/"))
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

// promptParts wraps a plain prompt as the single text part of a message.
func promptParts(text string) []provider.Part {
	return []provider.Part{{Type: provider.PartText, Text: text}}
}
