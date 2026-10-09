package httpapi_test

import (
	"net/http"
	"testing"
)

// TestLoginLimitChargesOnlyRejectedCredentials is the shared-NAT guard: a
// workshop room signs in from a single address, so a successful login must never
// spend the failure budget. Every attempt here is well past the old 5/min
// ceiling and must still succeed.
func TestLoginLimitChargesOnlyRejectedCredentials(t *testing.T) {
	h := newHarness(t)

	for attempt := 1; attempt <= 40; attempt++ {
		resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sign-in %d: status %d (%s)", attempt, resp.StatusCode, body)
		}
	}
}

// TestLoginLimitStopsGuessingPerClientIP covers the brute-force brake: rejected
// credentials spend one unit each, keyed by client IP. Every request opens a
// fresh TCP connection, so a limiter keyed on the remote address including the
// ephemeral port would never trigger (plans/3-security.md T4/T5).
func TestLoginLimitStopsGuessingPerClientIP(t *testing.T) {
	h := newHarness(t)

	for attempt := 1; attempt <= 30; attempt++ {
		client := newClient(t) // a distinct connection, same host
		resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "nope-not-a-token"}, client)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("guess %d: status %d (%s), want 401", attempt, resp.StatusCode, body)
		}
	}

	// The address is out of budget, so even the right credential waits it out:
	// the check runs before the comparison, or a guesser could keep hammering.
	client := newClient(t)
	resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, client)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("guess past the limit: status %d (%s), want 429", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Fatal("429 without a Retry-After header")
	}
}

// TestAPIHasNoPerIdentityLimit pins the deliberate removal of the blanket
// 30/min API limit: it keyed every authenticated request to one principal, so it
// throttled the operator during ordinary work while providing no brute-force
// protection at all.
func TestAPIHasNoPerIdentityLimit(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d (%s)", resp.StatusCode, body)
	}

	for i := range 60 {
		resp, body := h.do(http.MethodGet, "/api/rooms", nil, h.admin)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status %d (%s)", i+1, resp.StatusCode, body)
		}
	}
}
