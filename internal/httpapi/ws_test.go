package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/auth"
	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/core"
	"github.com/mark/beevibe/internal/db"
	"github.com/mark/beevibe/internal/files"
	"github.com/mark/beevibe/internal/httpapi"
)

type harness struct {
	t      *testing.T
	server *httptest.Server
	db     *sql.DB
	core   *core.Core
	admin  *http.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	handle, err := db.Open(context.Background(), filepath.Join(dir, "beevibe.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	cfg := &config.Config{
		AdminPassword:     "devpass",
		SessionSecret:     []byte("test-secret"),
		DataDir:           dir,
		Port:              "0",
		AgentModel:        "deepseek-flash",
		AgentMaxSteps:     20,
		AgentContextTurns: 50,
		MaxUsersPerRoom:   100,
		MaxRooms:          20,
		MaxConcurrentRuns: 8,
		PreviewDebounce:   10 * time.Millisecond,
		AgentRunTimeout:   time.Second,
		RendererURL:       "http://127.0.0.1:1",
		AppURL:            "http://127.0.0.1:8080",
	}
	q := db.Queries(handle)
	c := core.New(cfg, handle, q, files.New(dir), zap.NewNop())
	srv := httpapi.New(cfg, handle, q, auth.New(cfg.SessionSecret), c, nil, nil, zap.NewNop())
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	return &harness{t: t, server: ts, db: handle, core: c, admin: client}
}

func (h *harness) do(method, path string, body any, client *http.Client) (*http.Response, []byte) {
	h.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return resp, buf.Bytes()
}

func (h *harness) decode(method, path string, body any, client *http.Client, into any) *http.Response {
	h.t.Helper()
	resp, raw := h.do(method, path, body, client)
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			h.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return resp
}

// newClient returns an independent cookie jar (a separate browser).
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// TestWebSocketChatAppendEndToEnd covers plan step 3.4: an admin and a user
// connect over HTTP and WebSocket, and a room-wide broadcast reaches both.
func TestWebSocketChatAppendEndToEnd(t *testing.T) {
	h := newHarness(t)

	var login struct {
		Role string `json:"role"`
	}
	if resp := h.decode(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin, &login); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login: status %d", resp.StatusCode)
	}
	if login.Role != "admin" {
		t.Fatalf("admin login role = %q", login.Role)
	}

	var created struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	if resp := h.decode(http.MethodPost, "/api/rooms", map[string]any{"name": "Friday demo"}, h.admin, &created); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create room: status %d", resp.StatusCode)
	}
	roomID := created.Room.ID

	var userBody struct {
		User struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		} `json:"user"`
	}
	if resp := h.decode(http.MethodPost, "/api/rooms/"+roomID+"/users", map[string]any{"name": "Alice"}, h.admin, &userBody); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: status %d", resp.StatusCode)
	}

	user := newClient(t)
	var userLogin struct {
		Role   string `json:"role"`
		UserID *int64 `json:"userId"`
	}
	if resp := h.decode(http.MethodPost, "/api/login", map[string]string{"token": userBody.User.Token}, user, &userLogin); resp.StatusCode != http.StatusOK {
		t.Fatalf("user login: status %d", resp.StatusCode)
	}
	if userLogin.Role != "user" || userLogin.UserID == nil || *userLogin.UserID != userBody.User.ID {
		t.Fatalf("user login body = %+v", userLogin)
	}

	adminWS := h.dialWS(t, h.admin, "/ws/admin")
	defer adminWS.Close()
	userWS := h.dialWS(t, user, "/ws/user")
	defer userWS.Close()

	adminHello := readFrame(t, adminWS)
	if adminHello["type"] != "hello" || adminHello["role"] != "admin" {
		t.Fatalf("admin hello = %v", adminHello)
	}
	if adminHello["roomId"] != nil || adminHello["userId"] != nil {
		t.Fatalf("admin hello must carry null roomId/userId: %v", adminHello)
	}
	for _, key := range []string{"roomState", "agentState", "queueDepth", "blocked"} {
		if _, ok := adminHello[key]; !ok {
			t.Fatalf("admin hello is missing %q: %v", key, adminHello)
		}
	}

	userHello := readFrame(t, userWS)
	if userHello["type"] != "hello" || userHello["role"] != "user" {
		t.Fatalf("user hello = %v", userHello)
	}
	if userHello["roomId"] != roomID {
		t.Fatalf("user hello roomId = %v, want %q", userHello["roomId"], roomID)
	}
	if userHello["roomState"] != core.RoomOpen {
		t.Fatalf("user hello roomState = %v", userHello["roomState"])
	}
	if userHello["blocked"] != true {
		t.Fatalf("a user in an open (not started) room must be blocked: %v", userHello)
	}

	if resp := h.decode(http.MethodPost, "/api/rooms/"+roomID+"/broadcast", map[string]string{"text": "hello room"}, h.admin, nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("broadcast: status %d", resp.StatusCode)
	}

	for name, conn := range map[string]*websocket.Conn{"admin": adminWS, "user": userWS} {
		frame := readFrame(t, conn)
		if frame["type"] != "chat.append" {
			t.Fatalf("%s socket: expected chat.append, got %v", name, frame)
		}
		message, ok := frame["message"].(map[string]any)
		if !ok {
			t.Fatalf("%s socket: chat.append without message: %v", name, frame)
		}
		if message["text"] != "hello room" {
			t.Fatalf("%s socket: message text = %v", name, message["text"])
		}
		if message["kind"] != core.KindAdmin {
			t.Fatalf("%s socket: message kind = %v", name, message["kind"])
		}
	}
}

// TestWebSocketOriginAndRateLimit covers the T8 origin check and the 1008 close
// on exceeding 10 messages/s.
func TestWebSocketOriginAndRateLimit(t *testing.T) {
	h := newHarness(t)
	if resp := h.decode(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", resp.StatusCode)
	}

	// Foreign Origin must be rejected before the upgrade.
	header := http.Header{}
	header.Set("Origin", "https://evil.example")
	header.Set("Cookie", h.cookieHeader())
	wsURL := "ws" + h.server.URL[len("http"):] + "/ws/admin"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		conn.Close()
		t.Fatalf("handshake with a foreign Origin must fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Origin status = %v, want 403", resp)
	}

	// Per-connection rate limit: the 11th frame in one second closes with 1008.
	good := h.dialWS(t, h.admin, "/ws/admin")
	defer good.Close()
	readFrame(t, good)
	// More than 10 frames inside one second must trip the per-connection limit.
	for range 15 {
		if err := good.WriteJSON(map[string]string{"type": "ping"}); err != nil {
			break
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_ = good.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := good.ReadMessage(); err != nil {
			var closeErr *websocket.CloseError
			if !errors.As(err, &closeErr) {
				t.Fatalf("connection failed without a close frame: %v", err)
			}
			if closeErr.Code != websocket.ClosePolicyViolation {
				t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.ClosePolicyViolation)
			}
			return
		}
	}
	t.Fatalf("socket was not closed after exceeding 10 messages/s")
}

func (h *harness) dialWS(t *testing.T, client *http.Client, path string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Cookie", cookieHeaderFrom(t, client, h.server.URL))
	wsURL := "ws" + h.server.URL[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", path, err, status)
	}
	return conn
}

func (h *harness) cookieHeader() string {
	return cookieHeaderFrom(h.t, h.admin, h.server.URL)
}

func cookieHeaderFrom(t *testing.T, client *http.Client, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	jar, ok := client.Jar.(*cookiejar.Jar)
	if !ok {
		t.Fatalf("client has no cookie jar")
	}
	var parts []string
	for _, c := range jar.Cookies(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// TestWebSocketMicAndHelp covers the user→admin live signals: the mic indicator,
// the raise-hand badge, and that unknown frame types are ignored.
func TestWebSocketMicAndHelp(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login: %d (%s)", resp.StatusCode, body)
	}
	var created struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	if resp, body := h.do(http.MethodPost, "/api/rooms", map[string]any{"name": "demo"}, h.admin); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create room: %d (%s)", resp.StatusCode, body)
	} else if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	var userBody struct {
		User struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		} `json:"user"`
	}
	resp, body := h.do(http.MethodPost, "/api/rooms/"+created.Room.ID+"/users", map[string]any{"name": "Alice"}, h.admin)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: %d (%s)", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &userBody); err != nil {
		t.Fatalf("decode user: %v", err)
	}

	user := newClient(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": userBody.User.Token}, user); resp.StatusCode != http.StatusOK {
		t.Fatalf("user login: %d (%s)", resp.StatusCode, body)
	}

	adminWS := h.dialWS(t, h.admin, "/ws/admin")
	defer adminWS.Close()
	userWS := h.dialWS(t, user, "/ws/user")
	defer userWS.Close()
	readFrame(t, adminWS)
	readFrame(t, userWS)

	// A mic.state with a non-boolean payload is ignored, not fatal.
	if err := userWS.WriteJSON(map[string]any{"type": "mic.state", "on": "yes"}); err != nil {
		t.Fatalf("send malformed mic.state: %v", err)
	}
	// mic.state -> the admin's view of that user flips micOn.
	if err := userWS.WriteJSON(map[string]any{"type": "mic.state", "on": true}); err != nil {
		t.Fatalf("send mic.state: %v", err)
	}
	update := awaitUserUpdate(t, adminWS, userBody.User.ID)
	if update["micOn"] != true {
		t.Fatalf("micOn = %v, want true (%v)", update["micOn"], update)
	}

	// unknown types are ignored: no frame must arrive.
	if err := userWS.WriteJSON(map[string]any{"type": "totally.unknown"}); err != nil {
		t.Fatalf("send unknown frame: %v", err)
	}
	_ = userWS.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, data, err := userWS.ReadMessage(); err == nil {
		t.Fatalf("unknown frame produced a reply: %s", data)
	}

	// help.request -> a help chat entry plus helpPending on the admin row.
	if err := userWS.WriteJSON(map[string]any{"type": "help.request"}); err != nil {
		t.Fatalf("send help.request: %v", err)
	}
	sawChat, sawPending := false, false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !(sawChat && sawPending) {
		_ = adminWS.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, data, err := adminWS.ReadMessage()
		if err != nil {
			break
		}
		var frame map[string]any
		if err := json.Unmarshal(data, &frame); err != nil {
			t.Fatalf("decode frame %q: %v", data, err)
		}
		switch frame["type"] {
		case "chat.append":
			message, _ := frame["message"].(map[string]any)
			if message["kind"] != core.KindHelp {
				t.Fatalf("unexpected chat kind %v", message["kind"])
			}
			if message["text"] != core.MsgHelpRequest {
				t.Fatalf("help text = %v", message["text"])
			}
			sawChat = true
		case "user.update":
			if u, ok := frame["user"].(map[string]any); ok && u["helpPending"] == true {
				sawPending = true
			}
		}
	}
	if !sawChat || !sawPending {
		t.Fatalf("raise hand was not signalled to the admin (chat=%v pending=%v)", sawChat, sawPending)
	}

	// The persisted chat log carries the help entry for the user.
	var chat struct {
		Messages []core.MessageJSON `json:"messages"`
	}
	if resp := h.decode(http.MethodGet, "/api/me/messages", nil, user, &chat); resp.StatusCode != http.StatusOK {
		t.Fatalf("messages: %d", resp.StatusCode)
	}
	if len(chat.Messages) != 1 || chat.Messages[0].Kind != core.KindHelp {
		t.Fatalf("chat = %+v", chat.Messages)
	}
}

// awaitUserUpdate reads frames until an admin user.update for userID arrives.
func awaitUserUpdate(t *testing.T, conn *websocket.Conn, userID int64) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := readFrame(t, conn)
		if frame["type"] != "user.update" {
			continue
		}
		user, ok := frame["user"].(map[string]any)
		if !ok {
			continue
		}
		if id, ok := user["id"].(float64); ok && int64(id) == userID {
			return user
		}
	}
	t.Fatalf("no user.update for user %d", userID)
	return nil
}

// TestHelpToggleAndAdminClear covers the Help button as a toggle: raising twice
// must not post the entry twice, lowering must clear the flag silently, and the
// admin can clear it through its own route.
func TestHelpToggleAndAdminClear(t *testing.T) {
	h := newHarness(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": "devpass"}, h.admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login: %d (%s)", resp.StatusCode, body)
	}
	room := h.createRoom("help room")
	user, token := h.createUser(room, "Alice")

	userClient := newClient(t)
	if resp, body := h.do(http.MethodPost, "/api/login", map[string]string{"token": token}, userClient); resp.StatusCode != http.StatusOK {
		t.Fatalf("user login: %d (%s)", resp.StatusCode, body)
	}

	adminWS := h.dialWS(t, h.admin, "/ws/admin")
	defer adminWS.Close()
	userWS := h.dialWS(t, userClient, "/ws/user")
	defer userWS.Close()
	readFrame(t, adminWS)
	readFrame(t, userWS)

	// Raise.
	if err := userWS.WriteJSON(map[string]any{"type": "help.request"}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	awaitHelpPending(t, adminWS, user, true)
	h.expectHelpEntries(t, room, user, 1)

	// Raising again is idempotent: no second chat entry, flag stays set.
	if err := userWS.WriteJSON(map[string]any{"type": "help.request"}); err != nil {
		t.Fatalf("re-raise: %v", err)
	}
	h.expectHelpEntries(t, room, user, 1)

	// Lower from the user's toggle.
	if err := userWS.WriteJSON(map[string]any{"type": "help.request", "on": false}); err != nil {
		t.Fatalf("lower: %v", err)
	}
	awaitHelpPending(t, adminWS, user, false)
	h.expectHelpEntries(t, room, user, 1)

	// The admin can raise and clear it too.
	helpPath := "/api/rooms/" + room + "/users/" + strconv.FormatInt(user, 10) + "/help"
	if resp, body := h.do(http.MethodPost, helpPath, map[string]any{"pending": true}, h.admin); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin raise: %d (%s)", resp.StatusCode, body)
	}
	awaitHelpPending(t, adminWS, user, true)
	h.expectHelpEntries(t, room, user, 2)

	if resp, body := h.do(http.MethodPost, helpPath, map[string]any{"pending": false}, h.admin); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin clear: %d (%s)", resp.StatusCode, body)
	}
	awaitHelpPending(t, adminWS, user, false)
	h.expectHelpEntries(t, room, user, 2)
}

// awaitHelpPending reads admin frames until the user's helpPending flag matches.
func awaitHelpPending(t *testing.T, conn *websocket.Conn, userID int64, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := readFrame(t, conn)
		if frame["type"] != "user.update" {
			continue
		}
		u, ok := frame["user"].(map[string]any)
		if !ok {
			continue
		}
		if id, ok := u["id"].(float64); !ok || int64(id) != userID {
			continue
		}
		if u["helpPending"] == want {
			return
		}
	}
	t.Fatalf("helpPending never became %v for user %d", want, userID)
}

// expectHelpEntries asserts how many kind=help entries the chat holds.
func (h *harness) expectHelpEntries(t *testing.T, room string, userID int64, want int) {
	t.Helper()
	var chat struct {
		Messages []core.MessageJSON `json:"messages"`
	}
	path := "/api/rooms/" + room + "/users/" + strconv.FormatInt(userID, 10) + "/messages"
	if resp := h.decode(http.MethodGet, path, nil, h.admin, &chat); resp.StatusCode != http.StatusOK {
		t.Fatalf("messages: %d", resp.StatusCode)
	}
	got := 0
	for _, m := range chat.Messages {
		if m.Kind == core.KindHelp {
			got++
		}
	}
	if got != want {
		t.Fatalf("help chat entries = %d, want %d (%+v)", got, want, chat.Messages)
	}
}

// createRoom creates a room through the admin API and returns its id.
func (h *harness) createRoom(name string) string {
	h.t.Helper()
	var created struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	if resp, body := h.do(http.MethodPost, "/api/rooms", map[string]any{"name": name}, h.admin); resp.StatusCode != http.StatusCreated {
		h.t.Fatalf("create room: %d (%s)", resp.StatusCode, body)
	} else if err := json.Unmarshal(body, &created); err != nil {
		h.t.Fatalf("decode room: %v", err)
	}
	return created.Room.ID
}

// createUser creates a user through the admin API and returns (id, token).
func (h *harness) createUser(roomID, name string) (int64, string) {
	h.t.Helper()
	var created struct {
		User struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		} `json:"user"`
	}
	if resp, body := h.do(http.MethodPost, "/api/rooms/"+roomID+"/users", map[string]any{"name": name}, h.admin); resp.StatusCode != http.StatusCreated {
		h.t.Fatalf("create user: %d (%s)", resp.StatusCode, body)
	} else if err := json.Unmarshal(body, &created); err != nil {
		h.t.Fatalf("decode user: %v", err)
	}
	return created.User.ID, created.User.Token
}

// roomChatTexts returns the persisted chat lines of a room's user, oldest-first.
func (h *harness) roomChatTexts(roomID string, userID int64) []string {
	h.t.Helper()
	var chat struct {
		Messages []core.MessageJSON `json:"messages"`
	}
	path := "/api/rooms/" + roomID + "/users/" + strconv.FormatInt(userID, 10) + "/messages"
	if resp := h.decode(http.MethodGet, path, nil, h.admin, &chat); resp.StatusCode != http.StatusOK {
		h.t.Fatalf("messages: %d", resp.StatusCode)
	}
	out := make([]string, 0, len(chat.Messages))
	for _, m := range chat.Messages {
		out = append(out, m.Kind+": "+m.Text)
	}
	return out
}

func readFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("decode frame %q: %v", data, err)
	}
	return frame
}
