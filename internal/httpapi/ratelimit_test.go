package httpapi_test

import (
	"net/http"
	"testing"
)

// TestLoginLimitIsPerClientIPNotPerConnection guards the per-IP login limit
// (plans/3-security.md T4/T5): each request above opens a fresh TCP connection,
// so a limiter keyed on the remote address including the ephemeral port would
// never trigger.
func TestLoginLimitIsPerClientIPNotPerConnection(t *testing.T) {
	h := newHarness(t)

	for attempt := 1; attempt <= 5; attempt++ {
		client := newClient(t) // a distinct connection, same host
		resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, client)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d: status %d (%s)", attempt, resp.StatusCode, body)
		}
	}

	client := newClient(t)
	resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, client)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th login from the same IP: status %d (%s), want 429", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Fatalf("429 without a Retry-After header")
	}
}

// TestAPILimitIsPerIdentityNotPerConnection covers the 30/min API limit and the
// D10 exemptions.
func TestAPILimitIsPerIdentityNotPerConnection(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d (%s)", resp.StatusCode, body)
	}

	// 30 metered requests are allowed; /api/me is exempt, so it never consumes
	// budget and must keep working afterwards.
	for i := range 30 {
		resp, body := h.do(http.MethodGet, "/api/rooms", nil, h.admin)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("metered request %d: status %d (%s)", i+1, resp.StatusCode, body)
		}
	}
	if resp, body := h.do(http.MethodGet, "/api/rooms", nil, h.admin); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("request over the limit: status %d (%s), want 429", resp.StatusCode, body)
	}
	if resp, body := h.do(http.MethodGet, "/api/me", nil, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("exempt /api/me was rate limited: status %d (%s)", resp.StatusCode, body)
	}
}
