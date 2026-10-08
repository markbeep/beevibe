package core

import (
	"sync"
)

// Socket is one WebSocket connection. The transport (internal/httpapi) owns the
// concrete type; core only fans frames out to it.
type Socket interface {
	// Send enqueues v as one JSON frame, reporting false when the socket is
	// closed or its outbound buffer is full.
	Send(v any) bool
	// Close closes the connection with a normal close frame.
	Close()
	// IsAdmin reports whether this is an admin-channel connection.
	IsAdmin() bool
	// UserID is the owning user for a user-channel connection, 0 for admin.
	UserID() int64
}

// Hub is the set of live WebSocket connections.
type Hub struct {
	mu     sync.Mutex
	users  map[int64]map[Socket]struct{}
	admins map[Socket]struct{}
	closed bool
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{
		users:  make(map[int64]map[Socket]struct{}),
		admins: make(map[Socket]struct{}),
	}
}

// Add registers a connection.
func (h *Hub) Add(s Socket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if s.IsAdmin() {
		h.admins[s] = struct{}{}
		return
	}
	uid := s.UserID()
	if h.users[uid] == nil {
		h.users[uid] = make(map[Socket]struct{})
	}
	h.users[uid][s] = struct{}{}
}

// Remove unregisters a connection.
func (h *Hub) Remove(s Socket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.IsAdmin() {
		delete(h.admins, s)
		return
	}
	uid := s.UserID()
	if conns, ok := h.users[uid]; ok {
		delete(conns, s)
		if len(conns) == 0 {
			delete(h.users, uid)
		}
	}
}

// ToUser sends a frame to every connection of one user.
func (h *Hub) ToUser(userID int64, v any) {
	for _, s := range h.userSockets(userID) {
		s.Send(v)
	}
}

// ToAdmins sends a frame to every admin connection.
func (h *Hub) ToAdmins(v any) {
	h.mu.Lock()
	conns := make([]Socket, 0, len(h.admins))
	for s := range h.admins {
		conns = append(conns, s)
	}
	h.mu.Unlock()
	for _, s := range conns {
		s.Send(v)
	}
}

// CloseUser closes every connection of one user (kick / delete).
func (h *Hub) CloseUser(userID int64) {
	for _, s := range h.userSockets(userID) {
		s.Close()
	}
}

// Online reports whether the user has at least one live connection.
func (h *Hub) Online(userID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.users[userID]) > 0
}

// CloseAll closes every connection (graceful shutdown).
func (h *Hub) CloseAll() {
	h.mu.Lock()
	all := make([]Socket, 0, len(h.admins))
	for s := range h.admins {
		all = append(all, s)
	}
	for _, conns := range h.users {
		for s := range conns {
			all = append(all, s)
		}
	}
	h.users = make(map[int64]map[Socket]struct{})
	h.admins = make(map[Socket]struct{})
	h.closed = true
	h.mu.Unlock()
	for _, s := range all {
		s.Close()
	}
}

func (h *Hub) userSockets(userID int64) []Socket {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := make([]Socket, 0, len(h.users[userID]))
	for s := range h.users[userID] {
		conns = append(conns, s)
	}
	return conns
}
