package core

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/files"
)

// Core holds the application state shared by the HTTP handlers: the database,
// the file store, the WebSocket hub, the live-state registry and the agent and
// preview coordinators.
type Core struct {
	cfg   *config.Config
	db    *sql.DB
	q     *dbgen.Queries
	files *files.Store
	hub   *Hub
	reg   *Registry
	log   *zap.Logger

	mu       sync.RWMutex
	agents   Agents
	previews Previewer

	// rev is the monotonic site.updated counter. Any change reloads the iframe.
	rev atomic.Int64
}

// New returns a Core without the agent supervisor and preview coordinator
// attached. Attach must be called before the server starts serving.
func New(cfg *config.Config, handle *sql.DB, q *dbgen.Queries, store *files.Store, log *zap.Logger) *Core {
	return &Core{
		cfg:   cfg,
		db:    handle,
		q:     q,
		files: store,
		hub:   NewHub(),
		reg:   NewRegistry(),
		log:   log,
	}
}

// Attach wires the agent supervisor and the preview coordinator. It exists as a
// separate step because both need the Core (as their Events/callback target)
// before they can be constructed.
func (c *Core) Attach(agents Agents, previews Previewer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agents = agents
	c.previews = previews
}

// Hub exposes the connection registry to the transport layer.
func (c *Core) Hub() *Hub { return c.hub }

// Registry exposes the live per-user state.
func (c *Core) Registry() *Registry { return c.reg }

// Queries exposes the sqlc query set.
func (c *Core) Queries() *dbgen.Queries { return c.q }

// Files exposes the file store.
func (c *Core) Files() *files.Store { return c.files }

// Config exposes the parsed configuration.
func (c *Core) Config() *config.Config { return c.cfg }

// Logger exposes the process logger.
func (c *Core) Logger() *zap.Logger { return c.log }

// DB exposes the database handle (health checks, shutdown).
func (c *Core) DB() *sql.DB { return c.db }

// Shutdown stops the agent supervisor, closes every WebSocket and stops the
// preview worker.
func (c *Core) Shutdown() {
	if a := c.agentsOrNil(); a != nil {
		a.StopAll()
	}
	c.hub.CloseAll()
	if p := c.previewsOrNil(); p != nil {
		p.Close()
	}
}

func (c *Core) agentsOrNil() Agents {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.agents
}

func (c *Core) previewsOrNil() Previewer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.previews
}

// --- agent supervisor Events implementation ---------------------------------

// AgentStatus persists one tool-status line and pushes it to the chat.
func (c *Core) AgentStatus(roomID string, userID int64, text string) {
	ctx := context.Background()
	if _, err := c.InsertMessage(ctx, roomID, &userID, KindAgentStatus, text); err != nil {
		c.log.Error("core: persist agent status", zap.Error(err))
	}
}

// AgentState pushes the current agent state to the user and the admin grid.
func (c *Core) AgentState(roomID string, userID int64) {
	c.emitAgentState(context.Background(), roomID, userID)
}

// SiteUpdated tells the user's browser to reload the iframe.
func (c *Core) SiteUpdated(roomID string, userID int64) {
	c.emitSiteUpdated(userID)
}

// RunFinished refreshes the user's preview and their grid row.
func (c *Core) RunFinished(roomID string, userID int64) {
	ctx := context.Background()
	c.TriggerPreview(roomID, userID)
	c.emitAgentState(ctx, roomID, userID)
	c.EmitUserUpdate(ctx, roomID, userID)
}

// SystemMessage persists a canned system chat line.
func (c *Core) SystemMessage(roomID string, userID int64, text string) {
	ctx := context.Background()
	if _, err := c.InsertMessage(ctx, roomID, &userID, KindSystem, text); err != nil {
		c.log.Error("core: persist system message", zap.Error(err))
	}
}

// --- event helpers ----------------------------------------------------------

// EmitUserUpdate pushes the user's admin-scoped row to every admin socket.
func (c *Core) EmitUserUpdate(ctx context.Context, roomID string, userID int64) {
	u, err := c.q.GetUser(ctx, userID)
	if err != nil {
		return
	}
	c.hub.ToAdmins(frameUserUpdate{Type: "user.update", User: c.UserView(u)})
}

func (c *Core) emitAgentState(ctx context.Context, roomID string, userID int64) {
	frame := frameAgentState{
		Type:    "agent.state",
		State:   AgentIdle,
		Blocked: c.blockedFor(ctx, roomID, userID),
	}
	if a := c.agentsOrNil(); a != nil {
		state, depth := a.State(userID)
		frame.State = state
		frame.QueueDepth = depth
		frame.RunID = a.RunID(userID)
	}
	c.hub.ToUser(userID, frame)
}

func (c *Core) emitSiteUpdated(userID int64) {
	c.hub.ToUser(userID, frameSiteUpdated{Type: "site.updated", Rev: c.rev.Add(1)})
}

func (c *Core) emitRoomUpdate(ctx context.Context, room dbgen.Room) {
	view, err := c.RoomView(ctx, room)
	if err != nil {
		return
	}
	c.hub.ToAdmins(frameRoomUpdate{Type: "room.update", Room: view})
}

func (c *Core) emitRoomState(roomID string, state string) {
	frame := frameRoomState{Type: "room.state", State: state}
	ids, err := c.q.ListUsersByRoomForBulk(context.Background(), roomID)
	if err != nil {
		return
	}
	for _, id := range ids {
		c.hub.ToUser(id, frame)
	}
}

// blockedFor implements D13: a user may not prompt when the room is not
// started, while their site is being reset, or once their token limit is
// reached.
func (c *Core) blockedFor(ctx context.Context, roomID string, userID int64) bool {
	if c.reg.Resetting(userID) {
		return true
	}
	room, err := c.q.GetRoom(ctx, roomID)
	if err != nil {
		return true
	}
	if room.State != RoomStarted {
		return true
	}
	u, err := c.q.GetUser(ctx, userID)
	if err != nil {
		return true
	}
	return u.TokenLimit.Valid && u.TokensUsed >= u.TokenLimit.Int64
}

// PromptBlocked reports whether prompting is currently refused for the user and
// the canned system message that explains it (D5, D13).
func (c *Core) PromptBlocked(ctx context.Context, roomID string, userID int64) (bool, string) {
	room, err := c.q.GetRoom(ctx, roomID)
	if err != nil {
		return true, MsgRoomNotStarted
	}
	switch room.State {
	case RoomStarted:
		// keep going
	case RoomClosed, RoomArchived:
		return true, MsgRoomClosed
	default:
		return true, MsgRoomNotStarted
	}
	if c.reg.Resetting(userID) {
		return true, MsgResetStart
	}
	u, err := c.q.GetUser(ctx, userID)
	if err != nil {
		return true, MsgAgentError
	}
	if u.TokenLimit.Valid && u.TokensUsed >= u.TokenLimit.Int64 {
		return true, MsgTokenLimit
	}
	return false, ""
}

// PreviewImage returns the cached preview for a user.
func (c *Core) PreviewImage(userID int64) (data []byte, rev int64, ok bool) {
	if p := c.previewsOrNil(); p != nil {
		return p.Image(userID)
	}
	return nil, 0, false
}

// PreviewState reports ok | missing | failed for the user's tile.
func (c *Core) PreviewState(userID int64) string {
	if p := c.previewsOrNil(); p != nil {
		return p.State(userID)
	}
	return PreviewMissing
}

// ForcePreview requests an immediate re-capture (AOV-13).
func (c *Core) ForcePreview(roomID string, userID int64) {
	if p := c.previewsOrNil(); p != nil {
		p.Force(roomID, userID)
	}
}

// TriggerPreview requests a debounced capture.
func (c *Core) TriggerPreview(roomID string, userID int64) {
	if p := c.previewsOrNil(); p != nil {
		p.Trigger(roomID, userID)
	}
}

// DropPreview forgets a user's cached preview.
func (c *Core) DropPreview(userID int64) {
	if p := c.previewsOrNil(); p != nil {
		p.Drop(userID)
	}
}

// UserView converts a user row into its wire representation.
func (c *Core) UserView(u dbgen.User) UserJSON {
	state, depth := AgentIdle, 0
	if a := c.agentsOrNil(); a != nil {
		state, depth = a.State(u.ID)
	}
	preview := PreviewMissing
	if p := c.previewsOrNil(); p != nil {
		preview = p.State(u.ID)
	}
	return UserJSON{
		ID:           u.ID,
		RoomID:       u.RoomID,
		Name:         u.Name,
		Token:        u.Token,
		CreatedAt:    u.CreatedAt,
		KickedAt:     nullInt64(u.KickedAt),
		TokenLimit:   nullInt64(u.TokenLimit),
		TokensUsed:   u.TokensUsed,
		Online:       c.hub.Online(u.ID),
		MicOn:        c.reg.MicOn(u.ID),
		HelpPending:  c.reg.HelpPending(u.ID),
		AgentState:   state,
		QueueDepth:   depth,
		PreviewState: preview,
	}
}

func nullInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return new(v.Int64)
}

func nullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return new(v.String)
}
