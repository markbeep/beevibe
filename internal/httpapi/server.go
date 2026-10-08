package httpapi

import (
	"bufio"
	"context"
	"database/sql"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/auth"
	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/core"
	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/ratelimit"
	"github.com/mark/beevibe/internal/stt"
)

// RendererHealth reports whether the preview renderer is reachable.
type RendererHealth interface {
	Health(ctx context.Context) error
}

// Server holds everything the handlers need.
type Server struct {
	cfg      *config.Config
	db       *sql.DB
	q        *dbgen.Queries
	sessions *auth.Manager
	core     *core.Core
	stt      *stt.Transcriber
	renderer RendererHealth
	log      *zap.Logger

	loginLimiter *ratelimit.Limiter
	apiLimiter   *ratelimit.Limiter
}

// New returns a Server. transcriber and renderer may be nil only when the
// corresponding route is not exercised (tests).
func New(
	cfg *config.Config,
	handle *sql.DB,
	q *dbgen.Queries,
	sessions *auth.Manager,
	c *core.Core,
	transcriber *stt.Transcriber,
	renderer RendererHealth,
	log *zap.Logger,
) *Server {
	return &Server{
		cfg:          cfg,
		db:           handle,
		q:            q,
		sessions:     sessions,
		core:         c,
		stt:          transcriber,
		renderer:     renderer,
		log:          log,
		loginLimiter: ratelimit.New(time.Minute, 5),
		apiLimiter:   ratelimit.New(time.Minute, 30),
	}
}

// --- middleware -------------------------------------------------------------

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		fields := []zap.Field{
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", rec.status),
			zap.Duration("duration", time.Since(start)),
		}
		if sess, ok := auth.FromContext(r.Context()); ok {
			fields = append(fields, zap.String("role", string(sess.Role)))
			if sess.UserID != 0 {
				fields = append(fields, zap.Int64("userId", sess.UserID))
			}
			if sess.RoomID != "" {
				fields = append(fields, zap.String("roomId", sess.RoomID))
			}
		}
		s.log.Info("http", fields...)
	})
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("http: panic", zap.Any("panic", v), zap.String("path", r.URL.Path))
				if !rec.wrote {
					writeError(w, http.StatusInternalServerError, CodeInternal, "internal error")
				}
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.wrote {
		s.status = status
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Flush lets http.ServeContent stream.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets the WebSocket upgrader take over the connection; without it
// gorilla refuses the upgrade with a 500.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	s.wrote = true
	return h.Hijack()
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// --- origin / CSRF ----------------------------------------------------------

// requestOrigin reconstructs the browser-visible origin of the request.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(xfp, ",")[0]))
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// sameOrigin implements the CSRF rule of plans/api.md §1.
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
	default:
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		if !strings.EqualFold(strings.TrimRight(o, "/"), strings.TrimRight(requestOrigin(r), "/")) {
			return false
		}
	}
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
		if sfs != "same-origin" && sfs != "none" {
			return false
		}
	}
	return true
}

func (s *Server) withOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeError(w, http.StatusForbidden, CodeForbidden, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- rate limiting ----------------------------------------------------------

var previewPathRe = regexp.MustCompile(`^/api/rooms/[^/]+/users/[0-9]+/preview$`)

// isRateLimitExempt covers the idempotent, cheap, render-critical reads (D10).
func isRateLimitExempt(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.URL.Path == "/api/me" {
		return true
	}
	return previewPathRe.MatchString(r.URL.Path)
}

// identity keys the API limit per principal, using the cookie's signature only
// (no database round trip).
func (s *Server) identity(r *http.Request) string {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return ""
	}
	sess, err := s.sessions.Parse(c.Value)
	if err != nil {
		return ""
	}
	if sess.Role == auth.RoleAdmin {
		return "admin"
	}
	return "user:" + strconvItoa(sess.UserID)
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRateLimitExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		key := s.identity(r)
		if key == "" {
			// Unauthenticated requests are rejected by the auth middleware;
			// they are not charged to any principal.
			next.ServeHTTP(w, r)
			return
		}
		ok, retry := s.apiLimiter.Allow(key)
		if !ok {
			w.Header().Set("Retry-After", strconvItoa(int64(retry.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, CodeRateLimited, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP honours the reverse proxy's X-Forwarded-For. The remote address's
// ephemeral port must be stripped, otherwise every connection would form its own
// rate-limit bucket and the per-IP login limit would never trigger.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
