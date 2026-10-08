package httpapi

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// staticMIME is the explicit content-type table of plans/1-techstack.md §4.
var staticMIME = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".svg":  "image/svg+xml",
	".md":   "text/plain; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
}

// cspFor returns the UGC policy (D8): the frame's origin is opaque
// (sandbox="allow-scripts" without allow-same-origin), so 'self' would never
// match; the app's origin is used instead. No external network, no API calls.
func cspFor(r *http.Request) string {
	origin := requestOrigin(r)
	return strings.Join([]string{
		fmt.Sprintf("default-src %s data: 'unsafe-inline'", origin),
		fmt.Sprintf("script-src %s 'unsafe-inline'", origin),
		fmt.Sprintf("style-src %s 'unsafe-inline'", origin),
		fmt.Sprintf("img-src %s data:", origin),
		fmt.Sprintf("font-src %s data:", origin),
		"connect-src 'none'",
		fmt.Sprintf("frame-ancestors %s", origin),
	}, "; ")
}

// handleStaticRedirect points /rooms/{room}/{user} at its trailing-slash form so
// relative links inside the site resolve.
func (s *Server) handleStaticRedirect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	http.Redirect(w, r, fmt.Sprintf("/rooms/%s/%s/", vars["roomId"], vars["userId"]), http.StatusMovedPermanently)
}

// handleStatic serves one file from a user's site: public, no session check
// (T10), confined with os.Root (T2).
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomID := vars["roomId"]
	userID, err := parseUserID(vars["userId"])
	if err != nil || !validRoomID(roomID) {
		notFoundText(w)
		return
	}

	name := vars["path"]
	// Reject traversal outright; os.Root refuses escapes too, but the policy is
	// explicit and the answer must be 404, not a redirect.
	for _, elem := range strings.Split(name, "/") {
		if elem == ".." {
			notFoundText(w)
			return
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	if clean == "." || clean == "" {
		clean = "index.html"
	}

	root, err := s.core.Files().OpenUserExisting(roomID, userID)
	if err != nil {
		notFoundText(w)
		return
	}
	defer root.Close()

	f, err := root.Open(clean)
	if err != nil {
		notFoundText(w)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		notFoundText(w)
		return
	}
	if info.IsDir() {
		f.Close()
		clean = path.Join(clean, "index.html")
		f, err = root.Open(clean)
		if err != nil {
			notFoundText(w)
			return
		}
		defer f.Close()
		if info, err = f.Stat(); err != nil || info.IsDir() {
			notFoundText(w)
			return
		}
	}

	ct, ok := staticMIME[strings.ToLower(path.Ext(clean))]
	if !ok {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Reloaded after every tool call, so it must never be cached (D9).
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", cspFor(r))
	http.ServeContent(w, r, clean, info.ModTime(), f)
}

func notFoundText(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("not found\n"))
}

func validRoomID(id string) bool {
	if len(id) != 6 {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune("23456789ABCDEFGHJKMNPQRSTUVWXYZ", c) {
			return false
		}
	}
	return true
}

func parseUserID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid user id")
	}
	return id, nil
}
