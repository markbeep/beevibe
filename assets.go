// Package beevibe is the root package of the application. It exists so the
// built frontend can be embedded: //go:embed patterns may only reference files
// in the package's own directory, and web/dist lives here.
package beevibe

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:web/dist
var WebDist embed.FS

// devMIME pins the content types of the assets the SPA build emits.
var devMIME = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
	".png":   "image/png",
	".webp":  "image/webp",
	".woff2": "font/woff2",
	".map":   "application/json; charset=utf-8",
	".txt":   "text/plain; charset=utf-8",
}

// SPA serves the built single-page app: hashed assets with a long-lived
// immutable cache, and index.html for every client route.
func SPA() http.Handler {
	sub, err := fs.Sub(WebDist, "web/dist")
	if err != nil {
		// The embed pattern guarantees the directory exists.
		panic("beevibe: web/dist missing from the embedded FS: " + err.Error())
	}
	index, indexErr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if indexErr != nil {
			http.Error(w, "frontend not built: run 'just web-build'", http.StatusServiceUnavailable)
			return
		}
		name := path.Clean("/" + r.URL.Path)[1:]
		if name != "" && name != "index.html" {
			if f, err := sub.Open(name); err == nil {
				defer f.Close()
				if info, err := f.Stat(); err == nil && !info.IsDir() {
					serveAsset(w, r, name, f)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

func serveAsset(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	ext := strings.ToLower(path.Ext(name))
	ct, ok := devMIME[ext]
	if !ok {
		ct = mime.TypeByExtension(ext)
		if ct == "" {
			ct = "application/octet-stream"
		}
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	if seeker, ok := f.(io.ReadSeeker); ok {
		if info, err := f.Stat(); err == nil {
			http.ServeContent(w, r, name, info.ModTime(), seeker)
			return
		}
	}
	_, _ = io.Copy(w, f)
}
