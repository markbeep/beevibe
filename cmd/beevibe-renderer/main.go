// Command beevibe-renderer is the headless-Chromium screenshot service used by
// the beevibe preview pipeline (D16).
//
//	GET  /healthz          -> {"status":"ok","chromium":true} or 503
//	POST /capture {"url":} -> 200 image/webp, 400 malformed, 502 capture failed,
//	                          504 timed out
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// Fixed viewport (plan step 6.5).
const (
	viewportWidth      int64 = 1280
	viewportHeight     int64 = 720
	settleDelay              = 500 * time.Millisecond
	maxRequestBytes          = 8 << 10
	healthCacheTTL           = 5 * time.Second
	healthProbeTimeout       = 3 * time.Second
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	port := envOr("PORT", "9000")
	chromiumURL := envOr("CHROMIUM_URL", "http://127.0.0.1:9222")
	captureTimeout := time.Duration(envInt("CAPTURE_TIMEOUT_S", 15)) * time.Second
	quality := min(max(envInt("CAPTURE_QUALITY", 80), 0), 100)

	srv := &server{
		chromiumURL:    strings.TrimRight(chromiumURL, "/"),
		captureTimeout: captureTimeout,
		quality:        int64(quality),
		httpClient:     &http.Client{Timeout: healthProbeTimeout},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/capture", srv.handleCapture)

	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      captureTimeout + 15*time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("beevibe-renderer listening on %s (chromium %s)", httpSrv.Addr, srv.chromiumURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Fatalf("beevibe-renderer: %v", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("beevibe-renderer: shutdown: %v", err)
	}
}

type server struct {
	chromiumURL    string
	captureTimeout time.Duration
	quality        int64
	httpClient     *http.Client

	// captureMu serialises captures: one Chromium tab at a time.
	captureMu sync.Mutex

	healthMu sync.Mutex
	healthAt time.Time
	healthOK bool
}

// handleHealth reports whether Chromium answers on its debugging port. The
// result is cached briefly so tile refreshes do not hammer the browser.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.chromiumUp() {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok","chromium":true}`)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	io.WriteString(w, `{"status":"error","chromium":false}`)
}

// chromiumUp probes <CHROMIUM_URL>/json/version, honouring the cache TTL.
func (s *server) chromiumUp() bool {
	s.healthMu.Lock()
	if !s.healthAt.IsZero() && time.Since(s.healthAt) < healthCacheTTL {
		ok := s.healthOK
		s.healthMu.Unlock()
		return ok
	}
	s.healthMu.Unlock()

	ok := s.probeChromium()

	s.healthMu.Lock()
	s.healthAt = time.Now()
	s.healthOK = ok
	s.healthMu.Unlock()
	return ok
}

func (s *server) probeChromium() bool {
	ctx, cancel := context.WithTimeout(context.Background(), healthProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.chromiumURL+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode == http.StatusOK
}

type captureRequest struct {
	URL string `json:"url"`
}

func (s *server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErrorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var req captureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "malformed request body")
		return
	}
	if !validPageURL(req.URL) {
		writeErrorJSON(w, http.StatusBadRequest, "url must be an absolute http or https URL")
		return
	}

	// One capture at a time. The lock is released when the handler returns.
	s.captureMu.Lock()
	defer s.captureMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), s.captureTimeout)
	defer cancel()

	img, err := s.capture(ctx, req.URL)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeErrorJSON(w, http.StatusGatewayTimeout, "capture timed out")
			return
		}
		log.Printf("capture %s: %v", req.URL, err)
		writeErrorJSON(w, http.StatusBadGateway, "capture failed")
		return
	}

	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", strconv.Itoa(len(img)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(img); err != nil {
		log.Printf("capture %s: write response: %v", req.URL, err)
	}
}

// capture renders pageURL in its own tab and returns the raw WebP bytes. The
// tab context is cancelled on return, so a crashing page cannot wedge the
// service.
func (s *server) capture(ctx context.Context, pageURL string) ([]byte, error) {
	allocCtx, cancelAlloc := remote.NewAllocator(ctx, s.chromiumURL)
	defer cancelAlloc()

	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	quality := s.quality
	fromSurface := true

	if err := chromedp.Do(tabCtx,
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
				Width:             viewportWidth,
				Height:            viewportHeight,
				DeviceScaleFactor: 1,
				Mobile:            false,
			})
			return err
		}),
		chromedp.Navigate(pageURL),
		chromedp.Sleep(settleDelay),
	); err != nil {
		return nil, err
	}

	shot, err := chromedp.Run(tabCtx, func(ctx context.Context, t *chromedp.Target) (page.CaptureScreenshotResult, error) {
		return cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{
			Format:      page.CaptureScreenshotFormatWebp,
			Quality:     &quality,
			FromSurface: &fromSurface,
		})
	})
	if err != nil {
		return nil, err
	}
	if len(shot.Data) == 0 {
		return nil, errors.New("empty screenshot")
	}
	return shot.Data, nil
}

// validPageURL accepts only absolute http/https URLs.
func validPageURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if !u.IsAbs() || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func writeErrorJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
