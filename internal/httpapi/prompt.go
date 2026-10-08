package httpapi

import (
	"net/http"
	"strings"

	"github.com/mark/beevibe/internal/core"
)

// handlePrompt records the transcript and queues an agent run. The transcript is
// persisted even when the prompt is refused (D5).
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionOf(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Text string `json:"text"`
	}](w, r)
	if !ok {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "text is required")
		return
	}
	ctx := r.Context()
	roomID, userID := sess.RoomID, sess.UserID

	if _, err := s.core.InsertMessage(ctx, roomID, &userID, core.KindTranscript, text); err != nil {
		s.fail(w, err)
		return
	}

	if blocked, reason := s.core.PromptBlocked(ctx, roomID, userID); blocked {
		if _, err := s.core.InsertMessage(ctx, roomID, &userID, core.KindSystem, reason); err != nil {
			s.log.Warn("prompt: persist blocked notice failed")
		}
		writeError(w, http.StatusConflict, CodeConflict, reason)
		return
	}

	runID, queued, depth, err := s.core.EnqueuePrompt(ctx, roomID, userID, text)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"runId":      runID,
		"queued":     queued,
		"queueDepth": depth,
	})
}

// handleResetContext starts a new agent session for the signed-in user.
func (s *Server) handleResetContext(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionOf(w, r)
	if !ok {
		return
	}
	if err := s.core.ResetAgentContext(r.Context(), sess.RoomID, sess.UserID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
