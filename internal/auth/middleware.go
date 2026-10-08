package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mark/beevibe/internal/db/gen"
)

// RequireAdmin returns middleware that only admits a valid admin session.
func (m *Manager) RequireAdmin(handle *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := m.session(w, r)
			if !ok {
				return
			}
			if s.Role != RoleAdmin {
				writeAuthError(w, http.StatusForbidden, "forbidden", "admin session required")
				return
			}
			next.ServeHTTP(w, r.WithContext(withSession(r.Context(), s)))
		})
	}
}

// RequireUser returns middleware that admits a valid user session whose user
// and room rows still exist and whose room is not archived (D7).
func (m *Manager) RequireUser(handle *sql.DB) func(http.Handler) http.Handler {
	q := dbgen.New(handle)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := m.session(w, r)
			if !ok {
				return
			}
			if s.Role != RoleUser {
				writeAuthError(w, http.StatusForbidden, "forbidden", "user session required")
				return
			}
			user, err := q.GetUser(r.Context(), s.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeAuthError(w, http.StatusUnauthorized, "unauthorized", "unknown user")
					return
				}
				writeAuthError(w, http.StatusInternalServerError, "internal", "session lookup failed")
				return
			}
			if user.RoomID != s.RoomID {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "session does not match user")
				return
			}
			room, err := q.GetRoom(r.Context(), s.RoomID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeAuthError(w, http.StatusUnauthorized, "unauthorized", "unknown room")
					return
				}
				writeAuthError(w, http.StatusInternalServerError, "internal", "session lookup failed")
				return
			}
			if room.State == "archived" {
				writeAuthError(w, http.StatusForbidden, "forbidden", "room archived")
				return
			}
			next.ServeHTTP(w, r.WithContext(withSession(r.Context(), s)))
		})
	}
}

// Session returns the session carried by the request cookie without touching
// the database. Used by handlers that need the identity before a guarded route.
func (m *Manager) Session(r *http.Request) (Session, bool) {
	return m.session(nil, r)
}

func (m *Manager) session(w http.ResponseWriter, r *http.Request) (Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		if w != nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		}
		return Session{}, false
	}
	s, err := m.Parse(c.Value)
	if err != nil {
		if w != nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		}
		return Session{}, false
	}
	return s, true
}

func withSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// writeAuthError mirrors internal/httpapi's envelope writer. It is duplicated
// here because httpapi imports this package, so the dependency cannot reverse.
func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
