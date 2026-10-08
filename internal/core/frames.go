package core

// The WebSocket frames defined by plans/api.md §4. Every shape here is frozen.
// Unknown types must be ignored by clients, so new types may be added (D2).

type frameHello struct {
	Type       string  `json:"type"`
	Role       string  `json:"role"`
	RoomID     *string `json:"roomId"`
	UserID     *int64  `json:"userId"`
	RoomState  *string `json:"roomState"`
	AgentState string  `json:"agentState"`
	QueueDepth int     `json:"queueDepth"`
	Blocked    bool    `json:"blocked"`
}

type framePong struct {
	Type string `json:"type"`
}

type frameChatAppend struct {
	Type    string      `json:"type"`
	Message MessageJSON `json:"message"`
}

type frameAgentState struct {
	Type       string `json:"type"`
	State      string `json:"state"`
	QueueDepth int    `json:"queueDepth"`
	RunID      *int64 `json:"runId"`
	Blocked    bool   `json:"blocked"`
}

type frameSiteUpdated struct {
	Type string `json:"type"`
	Rev  int64  `json:"rev"`
}

type frameRoomState struct {
	Type  string `json:"type"`
	State string `json:"state"`
}

type frameUserUpdate struct {
	Type string   `json:"type"`
	User UserJSON `json:"user"`
}

type frameRoomUpdate struct {
	Type string   `json:"type"`
	Room RoomJSON `json:"room"`
}

// frameResetProgress drives the admin's room-wide reset progress bar (D2).
type frameResetProgress struct {
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Running bool   `json:"running"`
}

// HelloFrame builds the post-connect snapshot.
func HelloFrame(role string, roomID *string, userID *int64, roomState *string, agentState string, queueDepth int, blocked bool) any {
	return frameHello{
		Type:       "hello",
		Role:       role,
		RoomID:     roomID,
		UserID:     userID,
		RoomState:  roomState,
		AgentState: agentState,
		QueueDepth: queueDepth,
		Blocked:    blocked,
	}
}

// PongFrame answers a client ping.
func PongFrame() any { return framePong{Type: "pong"} }

// Reset scopes for frameResetProgress.
const (
	ResetScopeTemplate = "template"
	ResetScopeBulk     = "bulk"
)
