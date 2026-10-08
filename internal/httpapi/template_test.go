package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mark/beevibe/internal/core"
	"github.com/mark/beevibe/internal/db/gen"
)

// doRaw sends a request with an arbitrary body/content type (multipart uploads).
func (h *harness) doRaw(method, path string, body io.Reader, contentType string, client *http.Client) *http.Response {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, body)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func multipartBody(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := w.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func decodeBody(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
	}
}

// TestTemplateUploadReseedsUsers covers AREDIT-9/10/11: a template upload
// replaces every user's files, rejects the upload as a whole when one file
// breaks the policy, and reports progress over the admin channel (D2).
func TestTemplateUploadReseedsUsers(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d (%s)", resp.StatusCode, body)
	}

	var created struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	h.decode(http.MethodPost, "/api/rooms", map[string]any{"name": "template room"}, h.admin, &created)
	roomID := created.Room.ID

	var ids []int64
	for _, name := range []string{"Alice", "Bob"} {
		var userBody struct {
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		}
		h.decode(http.MethodPost, "/api/rooms/"+roomID+"/users", map[string]any{"name": name}, h.admin, &userBody)
		ids = append(ids, userBody.User.ID)
	}

	// Simulate agent-written content that the template upload must replace.
	for _, id := range ids {
		if err := h.core.Files().Write(roomID, id, "index.html", []byte("<h1>agent work</h1>")); err != nil {
			t.Fatalf("seed agent file: %v", err)
		}
	}

	adminWS := h.dialWS(t, h.admin, "/ws/admin")
	defer adminWS.Close()
	readFrame(t, adminWS)

	body, contentType := multipartBody(t, map[string]string{
		"index.html": "<h1>template</h1>",
		"style.css":  "body { color: red; }",
	})
	resp := h.doRaw(http.MethodPut, "/api/rooms/"+roomID+"/template", body, contentType, h.admin)
	var accepted struct {
		UsersReset int `json:"usersReset"`
	}
	decodeBody(t, resp, &accepted)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("template upload: %d", resp.StatusCode)
	}
	if accepted.UsersReset != len(ids) {
		t.Fatalf("usersReset = %d, want %d", accepted.UsersReset, len(ids))
	}

	// Progress must start and finish, covering every user.
	sawRunning, sawDone := false, false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !sawDone {
		_ = adminWS.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, data, err := adminWS.ReadMessage()
		if err != nil {
			break
		}
		var frame map[string]any
		if err := json.Unmarshal(data, &frame); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		if frame["type"] != "reset.progress" {
			continue
		}
		if frame["scope"] != core.ResetScopeTemplate {
			t.Fatalf("progress scope = %v", frame["scope"])
		}
		if total, ok := frame["total"].(float64); !ok || int(total) != len(ids) {
			t.Fatalf("progress total = %v", frame["total"])
		}
		if frame["running"] == true {
			sawRunning = true
		}
		if frame["running"] == false {
			if done, ok := frame["done"].(float64); !ok || int(done) != len(ids) {
				t.Fatalf("final progress done = %v", frame["done"])
			}
			sawDone = true
		}
	}
	if !sawRunning || !sawDone {
		t.Fatalf("reset.progress frames missing (running=%v done=%v)", sawRunning, sawDone)
	}

	// Every subdirectory now serves the template, not the agent's work.
	for _, id := range ids {
		index, err := h.core.Files().Read(roomID, id, "index.html")
		if err != nil {
			t.Fatalf("read index for %d: %v", id, err)
		}
		if string(index) != "<h1>template</h1>" {
			t.Fatalf("user %d index = %q", id, index)
		}
		css, err := h.core.Files().Read(roomID, id, "style.css")
		if err != nil {
			t.Fatalf("read style for %d: %v", id, err)
		}
		if string(css) != "body { color: red; }" {
			t.Fatalf("user %d style.css = %q", id, css)
		}
		if _, err := h.core.Files().Read(roomID, id, "README.md"); err != nil {
			t.Fatalf("README.md was not backfilled for %d: %v", id, err)
		}
	}

	// A single off-whitelist file rejects the whole upload and changes nothing.
	bad, badContentType := multipartBody(t, map[string]string{
		"index.html": "<h1>broken</h1>",
		"logo.png":   "not an image",
	})
	badResp := h.doRaw(http.MethodPut, "/api/rooms/"+roomID+"/template", bad, badContentType, h.admin)
	_ = badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("upload with logo.png: status %d, want 400", badResp.StatusCode)
	}
	index, err := h.core.Files().Read(roomID, ids[0], "index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if string(index) != "<h1>template</h1>" {
		t.Fatalf("a rejected upload changed the files: %q", index)
	}

	// The uploaded template survives deleting the template (subdirectories are
	// left alone) and the room view reports hasTemplate.
	var roomBody struct {
		Room struct {
			HasTemplate bool `json:"hasTemplate"`
		} `json:"room"`
	}
	h.decode(http.MethodGet, "/api/rooms/"+roomID, nil, h.admin, &roomBody)
	if !roomBody.Room.HasTemplate {
		t.Fatalf("hasTemplate = false after an upload")
	}
}

// TestUserResetDropsAgentHistory covers the per-user reset: it re-seeds the
// subdirectory, wipes the conversation history and tells the user (AREDIT-9).
func TestUserResetDropsAgentHistory(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d (%s)", resp.StatusCode, body)
	}
	var created struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	h.decode(http.MethodPost, "/api/rooms", map[string]any{"name": "reset room"}, h.admin, &created)
	roomID := created.Room.ID
	var userBody struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	h.decode(http.MethodPost, "/api/rooms/"+roomID+"/users", map[string]any{"name": "Alice"}, h.admin, &userBody)
	userID := userBody.User.ID

	// Pretend the agent ran: one history row and one written file.
	if _, err := h.core.Queries().AppendAgentMessage(context.Background(), dbgen.AppendAgentMessageParams{
		UserID:    userID,
		Seq:       1,
		Role:      "user",
		Content:   `[{"type":"text","text":"hello"}]`,
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("append history: %v", err)
	}
	if err := h.core.Files().Write(roomID, userID, "index.html", []byte("<h1>changed</h1>")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	resetPath := "/api/rooms/" + roomID + "/users/" + strconv.FormatInt(userID, 10) + "/reset"
	resp, raw := h.do(http.MethodPost, resetPath, nil, h.admin)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("reset: %d (%s)", resp.StatusCode, raw)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &status); err != nil || status.Status != "resetting" {
		t.Fatalf("reset body = %s", raw)
	}

	// Wait for the reset's *last* observable step. Waiting on the history wipe
	// instead would race: it happens between the reset-start and reset-done
	// messages.
	waitFor(t, 10*time.Second, func() bool {
		messages, err := h.core.ListUserChat(context.Background(), roomID, userID, 50, 0)
		if err != nil {
			return false
		}
		for _, m := range messages {
			if m.Text == core.MsgResetDone {
				return true
			}
		}
		return false
	}, "the reset never reported completion")

	rows, err := h.core.Queries().ListAgentMessages(context.Background(), userID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("agent history still holds %d rows", len(rows))
	}

	index, err := h.core.Files().Read(roomID, userID, "index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if string(index) == "<h1>changed</h1>" {
		t.Fatalf("the subdirectory was not re-seeded")
	}

	messages, err := h.core.ListUserChat(context.Background(), roomID, userID, 50, 0)
	if err != nil {
		t.Fatalf("list chat: %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("expected a reset start/done chat pair, got %+v", messages)
	}
	last := messages[len(messages)-1]
	if last.Text != core.MsgResetDone {
		t.Fatalf("last message = %q, want %q", last.Text, core.MsgResetDone)
	}
	start := messages[len(messages)-2]
	if start.Text != core.MsgResetStart {
		t.Fatalf("second to last message = %q, want %q", start.Text, core.MsgResetStart)
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s (after %s)", message, timeout)
}
