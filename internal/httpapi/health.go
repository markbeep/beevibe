package httpapi

import (
	"context"
	"net/http"
	"time"
)

// handleHealthz is a liveness probe: it checks no dependencies.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is a readiness probe: the database must ping and the preview
// renderer must be reachable.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, CodeInternal, "database unreachable")
		return
	}
	if s.renderer != nil {
		if err := s.renderer.Health(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, CodeInternal, "renderer unreachable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
