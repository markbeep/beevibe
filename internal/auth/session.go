// Package auth implements the HMAC-signed stateless session cookie (T16) and
// the middleware that guards the API and WebSocket routes.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// CookieName is the session cookie name.
const CookieName = "beevibe_session"

// ttl is how long a signed session stays valid.
const ttl = 7 * 24 * time.Hour

// Role identifies the two kinds of principal.
type Role string

const (
	// RoleAdmin is the single global administrator.
	RoleAdmin Role = "admin"
	// RoleUser is a room participant identified by their user row.
	RoleUser Role = "user"
)

// Session is the signed payload. It is the only thing the client holds; the
// referenced user and room are re-checked in the database on every request.
type Session struct {
	Role     Role   `json:"role"`
	RoomID   string `json:"roomId"`
	UserID   int64  `json:"userId"`
	IssuedAt int64  `json:"iat"`
}

// ErrInvalidSession is returned for a malformed, unsigned, tampered or expired
// cookie.
var ErrInvalidSession = errors.New("auth: invalid session")

// Manager signs and verifies session cookies with a fixed secret.
type Manager struct {
	secret []byte
}

// New returns a Manager keyed by secret.
func New(secret []byte) *Manager {
	return &Manager{secret: secret}
}

// Sign renders the session as "base64url(payload).base64url(hmac)".
func (m *Manager) Sign(s Session) string {
	if s.IssuedAt == 0 {
		s.IssuedAt = time.Now().Unix()
	}
	payload, err := json.Marshal(s)
	if err != nil {
		// Session is a fixed struct of JSON-safe fields.
		panic("auth: marshal session: " + err.Error())
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Parse verifies the signature and expiry of a cookie value.
func (m *Manager) Parse(v string) (Session, error) {
	var s Session
	payloadB64, sigB64, ok := strings.Cut(v, ".")
	if !ok || payloadB64 == "" || sigB64 == "" {
		return s, ErrInvalidSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return s, ErrInvalidSession
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return s, ErrInvalidSession
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return s, ErrInvalidSession
	}
	if err := json.Unmarshal(payload, &s); err != nil {
		return s, ErrInvalidSession
	}
	if s.Role != RoleAdmin && s.Role != RoleUser {
		return s, ErrInvalidSession
	}
	if time.Since(time.Unix(s.IssuedAt, 0)) > ttl {
		return s, ErrInvalidSession
	}
	return s, nil
}

// Set writes the session cookie.
func (m *Manager) Set(w http.ResponseWriter, r *http.Request, s Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    m.Sign(s),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   IsSecure(r),
		MaxAge:   int(ttl.Seconds()),
	})
}

// Clear expires the session cookie.
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// IsSecure reports whether the request arrived over TLS (terminated by the
// reverse proxy).
func IsSecure(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// contextKey is the private key type for the session in the request context.
type contextKey struct{}

// FromContext returns the session stored by the middleware.
func FromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(contextKey{}).(Session)
	return s, ok
}
