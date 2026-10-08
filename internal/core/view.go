// Package core holds the application logic shared by the HTTP handlers: rooms,
// users, messages, the file store, the WebSocket hub and the agent/preview
// integration points.
package core

// RoomJSON is the wire representation of a room (plans/api.md §2).
type RoomJSON struct {
	ID          string  `json:"id"`
	Name        *string `json:"name"`
	State       string  `json:"state"`
	Model       string  `json:"model"`
	UserCount   int64   `json:"userCount"`
	HasTemplate bool    `json:"hasTemplate"`
	CreatedAt   int64   `json:"createdAt"`
	StartedAt   *int64  `json:"startedAt"`
	ClosedAt    *int64  `json:"closedAt"`
	ArchivedAt  *int64  `json:"archivedAt"`
}

// UserJSON is the wire representation of a user. It is always admin-scoped;
// `token` is included deliberately so the admin can distribute it.
type UserJSON struct {
	ID           int64  `json:"id"`
	RoomID       string `json:"roomId"`
	Name         string `json:"name"`
	Token        string `json:"token"`
	CreatedAt    int64  `json:"createdAt"`
	KickedAt     *int64 `json:"kickedAt"`
	TokenLimit   *int64 `json:"tokenLimit"`
	TokensUsed   int64  `json:"tokensUsed"`
	Online       bool   `json:"online"`
	MicOn        bool   `json:"micOn"`
	HelpPending  bool   `json:"helpPending"`
	AgentState   string `json:"agentState"`
	QueueDepth   int    `json:"queueDepth"`
	PreviewState string `json:"previewState"`
}

// MessageJSON is a chat entry as delivered over REST and WebSocket.
type MessageJSON struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// StatsJSON is the per-user usage/derived-value block.
type StatsJSON struct {
	TokensUsed           int64  `json:"tokensUsed"`
	TokenLimit           *int64 `json:"tokenLimit"`
	LOC                  int    `json:"loc"`
	SitePath             string `json:"sitePath"`
	AgentContextMessages int    `json:"agentContextMessages"`
}

// Message kinds. The database CHECK constraint mirrors this set.
const (
	KindTranscript  = "transcript"
	KindAgentStatus = "agent_status"
	KindAdmin       = "admin"
	KindSystem      = "system"
	KindHelp        = "help"
)

// Room states.
const (
	RoomOpen     = "open"
	RoomStarted  = "started"
	RoomClosed   = "closed"
	RoomArchived = "archived"
)

// Agent states.
const (
	AgentIdle     = "idle"
	AgentQueued   = "queued"
	AgentThinking = "thinking"
	AgentEditing  = "editing"
	AgentError    = "error"
)

// Preview states.
const (
	PreviewOK      = "ok"
	PreviewMissing = "missing"
	PreviewFailed  = "failed"
)

// Canned chat messages, verbatim from the plan's literal table.
const (
	MsgUnintelligible = "I didn't catch that — please say it again."
	MsgCancelled      = "Prompt cancelled."
	MsgTimedOut       = "The agent timed out — please try again."
	MsgAgentError     = "Something went wrong — please try again."
	MsgTokenLimit     = "Token limit reached — asking the admin to raise it."
	MsgRoomNotStarted = "The room hasn't started yet."
	MsgRoomClosed     = "The room is closed — editing is disabled."
	MsgResetStart     = "Your site is being reset…"
	MsgResetDone      = "Your site has been reset."
	MsgSTTFailed      = "Couldn't transcribe that — please try again."
	MsgNewSession     = "New agent session — the agent forgot the earlier conversation."
	MsgHelpRequest    = "Needs help."

	// Room lifecycle notices. The close notice is part of the frozen literal
	// table; these are its additive counterparts so the chat shows when editing
	// becomes available again, not only when it stops.
	MsgRoomStarted  = "The room has started — you can edit your site."
	MsgRoomReopened = "The room is open again — you can edit your site."
)
