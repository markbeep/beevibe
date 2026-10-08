package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// captureTimeout bounds a single POST /capture round trip. The renderer itself
// gives up earlier (CAPTURE_TIMEOUT_S); this is the outer safety net.
const captureTimeout = 20 * time.Second

// maxImageBytes caps how much of a renderer response Capture will read.
const maxImageBytes = 16 << 20

// Client talks to the headless-Chromium renderer service (D16).
type Client struct {
	baseURL string
	http    *http.Client

	// AppURL is the base URL the app is reachable at from the renderer. It is
	// set from Config.AppURL by NewManager and used by PageURL. It carries no
	// trailing slash.
	AppURL string
}

// NewClient returns a Client for the renderer at baseURL (Config.RendererURL),
// without a trailing slash.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: captureTimeout},
	}
}

// Capture asks the renderer for a WebP screenshot of pageURL.
func (c *Client) Capture(ctx context.Context, pageURL string) ([]byte, error) {
	body, err := json.Marshal(struct {
		URL string `json:"url"`
	}{URL: pageURL})
	if err != nil {
		return nil, fmt.Errorf("preview: encode capture request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/capture", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("preview: build capture request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("preview: capture request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("preview: renderer returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/webp") {
		return nil, fmt.Errorf("preview: renderer returned content type %q, want image/webp", ct)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("preview: read capture response: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("preview: capture response larger than %d bytes", maxImageBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("preview: renderer returned an empty image")
	}
	return data, nil
}

// Health checks the renderer's /healthz endpoint. A non-200 status is an error.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("preview: build health request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("preview: health request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("preview: renderer health returned %d", resp.StatusCode)
	}
	return nil
}

// PageURL is the absolute URL the renderer navigates to for a user's site.
func (c *Client) PageURL(roomID string, userID int64) string {
	return c.AppURL + "/rooms/" + roomID + "/" + strconv.FormatInt(userID, 10) + "/"
}
