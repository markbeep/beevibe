package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mark/beevibe/internal/auth"
	"github.com/mark/beevibe/internal/core"
)

// wsInbound is the envelope of every client frame; unknown types are ignored.
type wsInbound struct {
	Type string `json:"type"`
	On   *bool  `json:"on"`
}

type wsClose struct {
	code int
	text string
}

// wsConn is one WebSocket connection. A single writer goroutine owns the
// underlying socket, which is what gorilla requires.
type wsConn struct {
	conn    *websocket.Conn
	send    chan []byte
	closeCh chan wsClose
	admin   bool
	userID  int64

	mu     sync.Mutex
	closed bool
}

func newWSConn(conn *websocket.Conn, admin bool, userID int64) *wsConn {
	return &wsConn{
		conn:    conn,
		send:    make(chan []byte, 64),
		closeCh: make(chan wsClose, 1),
		admin:   admin,
		userID:  userID,
	}
}

// Send enqueues one JSON frame. A full buffer means the client is too slow to
// keep up with the event stream, so the connection is closed rather than
// silently dropping frames.
func (c *wsConn) Send(v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- b:
		return true
	default:
		c.closed = true
		c.requestClose(websocket.ClosePolicyViolation, "client is not reading")
		return false
	}
}

// Close closes the connection with a normal close frame.
func (c *wsConn) Close() { c.closeWith(websocket.CloseNormalClosure, "") }

func (c *wsConn) closeWith(code int, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.requestClose(code, text)
}

// requestClose must be called with c.mu held.
func (c *wsConn) requestClose(code int, text string) {
	select {
	case c.closeCh <- wsClose{code: code, text: text}:
	default:
	}
}

// IsAdmin reports the channel kind.
func (c *wsConn) IsAdmin() bool { return c.admin }

// UserID is the owning user, 0 for the admin channel.
func (c *wsConn) UserID() int64 { return c.userID }

func (c *wsConn) writeLoop() {
	defer c.conn.Close()
	for {
		select {
		case b := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case req := <-c.closeCh:
			deadline := time.Now().Add(3 * time.Second)
			_ = c.conn.SetWriteDeadline(deadline)
			_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(req.code, req.text))
			return
		}
	}
}

// wsOriginOK verifies the handshake's Origin against the request host (T8).
func wsOriginOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser client (tests, tooling): nothing to compare against.
		return true
	}
	return strings.EqualFold(strings.TrimRight(origin, "/"), strings.TrimRight(requestOrigin(r), "/"))
}

// handleWS serves both realtime channels. The session middleware has already
// authenticated the cookie and the room state.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !wsOriginOK(r) {
		writeError(w, http.StatusForbidden, CodeForbidden, "cross-origin websocket rejected")
		return
	}
	sess, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not signed in")
		return
	}
	admin := sess.Role == auth.RoleAdmin

	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     wsOriginOK,
	}
	raw, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote a response.
		return
	}
	conn := newWSConn(raw, admin, sess.UserID)
	raw.SetReadLimit(1 << 20)

	hub := s.core.Hub()
	hub.Add(conn)
	defer hub.Remove(conn)
	go conn.writeLoop()
	defer conn.Close()

	conn.Send(s.helloFrame(r.Context(), sess, admin))

	s.readLoop(conn, sess, admin)
}

func (s *Server) readLoop(conn *wsConn, sess auth.Session, admin bool) {
	var windowStart = time.Now()
	var count int

	for {
		_, data, err := conn.conn.ReadMessage()
		if err != nil {
			return
		}
		// 10 messages per second per connection (plans/3-security.md §2).
		now := time.Now()
		if now.Sub(windowStart) >= time.Second {
			windowStart = now
			count = 0
		}
		count++
		if count > 10 {
			conn.closeWith(websocket.ClosePolicyViolation, "rate limit exceeded")
			return
		}

		var in wsInbound
		if err := json.Unmarshal(data, &in); err != nil {
			continue // malformed frames are ignored
		}
		ctx := context.Background()
		switch in.Type {
		case "ping":
			conn.Send(core.PongFrame())
		case "mic.state":
			if admin || in.On == nil {
				continue
			}
			s.core.Registry().SetMic(sess.UserID, *in.On)
			s.core.EmitUserUpdate(ctx, sess.RoomID, sess.UserID)
		case "help.request":
			if admin {
				continue
			}
			// The frozen frame is `{"type":"help.request"}` (raise); the optional
			// `on` makes the button a toggle. Absent means "raise".
			on := true
			if in.On != nil {
				on = *in.On
			}
			if err := s.core.SetHelp(ctx, sess.RoomID, sess.UserID, on); err != nil {
				s.log.Warn("ws: help toggle failed")
			}
		case "agent.cancel":
			if admin {
				continue
			}
			// Fire-and-forget: the channel has no reply for a no-op cancel.
			if err := s.core.CancelUser(ctx, sess.RoomID, sess.UserID); err != nil {
				s.log.Debug("ws: cancel ignored")
			}
		default:
			// Unknown types MUST be ignored.
		}
	}
}

// helloFrame is the post-connect snapshot the client re-syncs from.
func (s *Server) helloFrame(ctx context.Context, sess auth.Session, admin bool) any {
	if admin {
		return core.HelloFrame("admin", nil, nil, nil, core.AgentIdle, 0, false)
	}
	state := ""
	if room, err := s.core.GetRoom(ctx, sess.RoomID); err == nil {
		state = room.State
	}
	agentState, depth := core.AgentIdle, 0
	if st := s.core.AgentStateOf(sess.UserID); st != "" {
		agentState = st
	}
	depth = s.core.QueueDepthOf(sess.UserID)
	blocked, _ := s.core.PromptBlocked(ctx, sess.RoomID, sess.UserID)
	roomID, userID := sess.RoomID, sess.UserID
	return core.HelloFrame("user", &roomID, &userID, &state, agentState, depth, blocked)
}
