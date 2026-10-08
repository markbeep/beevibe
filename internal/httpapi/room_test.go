package httpapi_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/mark/beevibe/internal/core"
)

// TestRoomLifecycleNoticesAppearInChat covers the room-wide system lines: the
// frozen close notice plus its additive start/reopen counterparts, so a user's
// chat says when editing becomes available again and not only when it stops.
func TestRoomLifecycleNoticesAppearInChat(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d (%s)", resp.StatusCode, body)
	}
	room := h.createRoom("lifecycle")
	user, _ := h.createUser(room, "Alice")

	if got := h.roomChatTexts(room, user); len(got) != 0 {
		t.Fatalf("a fresh room already has chat lines: %v", got)
	}

	for _, step := range []struct {
		action string
		want   string
	}{
		{"start", core.MsgRoomStarted},
		{"close", core.MsgRoomClosed},
		{"reopen", core.MsgRoomReopened},
	} {
		resp, body := h.do(http.MethodPost, "/api/rooms/"+room+"/"+step.action, nil, h.admin)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d (%s)", step.action, resp.StatusCode, body)
		}
		if got := h.roomChatTexts(room, user); !slices.Contains(got, core.KindSystem+": "+step.want) {
			t.Fatalf("after %s the chat is %v, want it to contain %q", step.action, got, step.want)
		}
	}
}

// TestClosedRoomKeepsServingChat is the precondition the notice above relies on:
// closing a room blocks new logins and prompts, but a session opened while the
// room was open keeps reading the chat that explains why.
func TestClosedRoomKeepsServingChat(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d (%s)", resp.StatusCode, body)
	}
	room := h.createRoom("closed reads")
	_, token := h.createUser(room, "Bob")
	if resp, body := h.do(http.MethodPost, "/api/rooms/"+room+"/start", nil, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("start: %d (%s)", resp.StatusCode, body)
	}

	userClient := newClient(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": token}, userClient); resp.StatusCode != http.StatusOK {
		t.Fatalf("user login: %d (%s)", resp.StatusCode, body)
	}
	if resp, body := h.do(http.MethodPost, "/api/rooms/"+room+"/close", nil, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("close: %d (%s)", resp.StatusCode, body)
	}

	var chat struct {
		Messages []core.MessageJSON `json:"messages"`
	}
	if resp := h.decode(http.MethodGet, "/api/me/messages", nil, userClient, &chat); resp.StatusCode != http.StatusOK {
		t.Fatalf("chat read after close: %d, want 200", resp.StatusCode)
	}
	found := false
	for _, m := range chat.Messages {
		if m.Kind == core.KindSystem && m.Text == core.MsgRoomClosed {
			found = true
		}
	}
	if !found {
		t.Fatalf("close notice missing from the chat: %+v", chat.Messages)
	}

	// A brand-new login is refused while the room is closed (D7).
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": token}, newClient(t)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login into a closed room: %d (%s), want 403", resp.StatusCode, body)
	}
}
