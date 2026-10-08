package httpapi

import (
	"net/http"
	"strconv"
)

// handlePreview serves the cached WebP for one tile. A 404 means "no usable
// image", so the tile falls back to the colour tile (AOV-12).
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if _, err := s.core.GetUserInRoom(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	data, rev, found := s.core.PreviewImage(userID)
	if !found {
		writeError(w, http.StatusNotFound, CodeNotFound, "no preview available")
		return
	}
	etag := `"` + strconv.FormatInt(rev, 10) + `"`
	w.Header().Set("ETag", etag)
	// no-store is frozen in plans/api.md, so the client must revalidate with an
	// explicit If-None-Match (D1).
	w.Header().Set("Cache-Control", "no-store")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handlePreviewRefresh forces an immediate re-capture (AOV-13).
func (s *Server) handlePreviewRefresh(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if _, err := s.core.GetUserInRoom(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	s.core.ForcePreview(roomID, userID)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "refreshing"})
}

// matchesETag implements the weak comparison a browser would do.
func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, candidate := range splitList(header) {
		if candidate == etag || candidate == "W/"+etag || candidate == "*" {
			return true
		}
	}
	return false
}

func splitList(v string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(v); i++ {
		if i == len(v) || v[i] == ',' {
			part := trimSpace(v[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(v string) string {
	start, end := 0, len(v)
	for start < end && (v[start] == ' ' || v[start] == '\t') {
		start++
	}
	for end > start && (v[end-1] == ' ' || v[end-1] == '\t') {
		end--
	}
	return v[start:end]
}
