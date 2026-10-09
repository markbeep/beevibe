package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/auth"
	"github.com/mark/beevibe/internal/core"
)

type sessionBody struct {
	Role      string          `json:"role"`
	RoomID    *string         `json:"roomId"`
	UserID    *int64          `json:"userId"`
	Name      *string         `json:"name"`
	RoomState *string         `json:"roomState"`
	Stats     *core.StatsJSON `json:"stats,omitempty"`
}

type loginRequest struct {
	Token string `json:"token"`
}

// handleLogin accepts the admin password or a user token in the same field.
// Only rejected credentials spend the per-IP budget, so a room signing in
// together never limits itself (see loginFailWindow).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if blocked, retry := s.loginLimiter.Exceeded(ip); blocked {
		w.Header().Set("Retry-After", strconvItoa(int64(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, CodeRateLimited, "too many login attempts")
		return
	}
	req, ok := decodeJSON[loginRequest](w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	if constantTimeEqual(req.Token, s.cfg.AdminPassword) {
		s.sessions.Set(w, r, auth.Session{Role: auth.RoleAdmin})
		name := "Admin"
		writeJSON(w, http.StatusOK, sessionBody{Role: string(auth.RoleAdmin), Name: &name})
		return
	}

	user, err := s.q.GetUserByToken(ctx, req.Token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.loginLimiter.Allow(ip)
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid token")
			return
		}
		s.log.Error("login: user lookup failed")
		writeError(w, http.StatusInternalServerError, CodeInternal, "login failed")
		return
	}
	room, err := s.q.GetRoom(ctx, user.RoomID)
	if err != nil {
		s.loginLimiter.Allow(ip)
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid token")
		return
	}
	// D7: a closed or archived room cannot be entered.
	if room.State == core.RoomClosed || room.State == core.RoomArchived {
		writeError(w, http.StatusForbidden, CodeForbidden, "room is not open")
		return
	}
	s.sessions.Set(w, r, auth.Session{Role: auth.RoleUser, RoomID: user.RoomID, UserID: user.ID})
	if err := s.core.TouchUserLastSeen(ctx, user.ID); err != nil {
		s.log.Warn("login: touch last seen failed")
	}
	roomID, state, name := user.RoomID, room.State, user.Name
	userID := user.ID
	writeJSON(w, http.StatusOK, sessionBody{
		Role:      string(auth.RoleUser),
		RoomID:    &roomID,
		UserID:    &userID,
		Name:      &name,
		RoomState: &state,
	})
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleMe rebuilds the client's session on load and reconnect.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not signed in")
		return
	}
	sess, err := s.sessions.Parse(c.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid session")
		return
	}
	ctx := r.Context()

	if sess.Role == auth.RoleAdmin {
		name := "Admin"
		writeJSON(w, http.StatusOK, sessionBody{Role: string(auth.RoleAdmin), Name: &name})
		return
	}

	user, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "unknown user")
		return
	}
	room, err := s.q.GetRoom(ctx, user.RoomID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "unknown room")
		return
	}
	// D7: an archived room ejects the user back to the index page.
	if room.State == core.RoomArchived {
		writeError(w, http.StatusForbidden, CodeForbidden, "room archived")
		return
	}
	stats, err := s.core.StatsFor(ctx, user.RoomID, user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.core.TouchUserLastSeen(ctx, user.ID); err != nil {
		s.log.Warn("me: touch last seen failed")
	}
	roomID, state, name, userID := user.RoomID, room.State, user.Name, user.ID
	writeJSON(w, http.StatusOK, sessionBody{
		Role:      string(auth.RoleUser),
		RoomID:    &roomID,
		UserID:    &userID,
		Name:      &name,
		RoomState: &state,
		Stats:     &stats,
	})
}

// decodeJSON decodes a request body, answering 400 when it is malformed.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "malformed request body")
		return v, false
	}
	return v, true
}

// fail maps a core error onto the frozen status/code pair.
func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrBadRequest):
		writeError(w, http.StatusBadRequest, CodeBadRequest, coreMessage(err))
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, CodeNotFound, coreMessage(err))
	case errors.Is(err, core.ErrConflict):
		writeError(w, http.StatusConflict, CodeConflict, coreMessage(err))
	default:
		s.log.Error("http: request failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal error")
	}
}

func coreMessage(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"not found: ", "conflict: ", "bad request: "} {
		if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
			return msg[len(prefix):]
		}
	}
	return msg
}

// constantTimeEqual compares two secrets without leaking their content.
func constantTimeEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
