package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/ids"
)

// CreateRoom allocates a room, subject to MAX_ROOMS.
func (c *Core) CreateRoom(ctx context.Context, name *string) (dbgen.Room, error) {
	count, err := c.q.CountRooms(ctx)
	if err != nil {
		return dbgen.Room{}, err
	}
	if count >= int64(c.cfg.MaxRooms) {
		return dbgen.Room{}, fmt.Errorf("%w: room limit reached (%d)", ErrConflict, c.cfg.MaxRooms)
	}
	var nn sql.NullString
	if name != nil {
		nn = sql.NullString{String: *name, Valid: true}
	}
	now := time.Now().Unix()
	var lastErr error
	for range 10 {
		room, err := c.q.CreateRoom(ctx, dbgen.CreateRoomParams{
			ID:        ids.RoomID(),
			Name:      nn,
			Model:     c.cfg.AgentModel,
			CreatedAt: now,
		})
		if err == nil {
			c.emitRoomUpdate(ctx, room)
			return room, nil
		}
		if !isUniqueViolation(err) {
			return dbgen.Room{}, err
		}
		lastErr = err
	}
	return dbgen.Room{}, fmt.Errorf("%w: could not allocate a room id: %v", ErrConflict, lastErr)
}

// PushRoomUpdate re-broadcasts a room's admin view (used after template edits,
// which change the derived hasTemplate field).
func (c *Core) PushRoomUpdate(ctx context.Context, roomID string) {
	room, err := c.GetRoom(ctx, roomID)
	if err != nil {
		return
	}
	c.emitRoomUpdate(ctx, room)
}

// GetRoom returns one room, or an error wrapping ErrNotFound.
func (c *Core) GetRoom(ctx context.Context, id string) (dbgen.Room, error) {
	room, err := c.q.GetRoom(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dbgen.Room{}, fmt.Errorf("%w: unknown room %q", ErrNotFound, id)
	}
	return room, err
}

// ListRooms returns every room, newest first.
func (c *Core) ListRooms(ctx context.Context) ([]RoomJSON, error) {
	rooms, err := c.q.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoomJSON, 0, len(rooms))
	for _, room := range rooms {
		view, err := c.RoomView(ctx, room)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// RoomView adds the derived fields (user count, template presence).
func (c *Core) RoomView(ctx context.Context, room dbgen.Room) (RoomJSON, error) {
	count, err := c.q.CountUsersByRoom(ctx, room.ID)
	if err != nil {
		return RoomJSON{}, err
	}
	return RoomJSON{
		ID:          room.ID,
		Name:        nullString(room.Name),
		State:       room.State,
		Model:       room.Model,
		UserCount:   count,
		HasTemplate: c.files.HasTemplate(room.ID),
		CreatedAt:   room.CreatedAt,
		StartedAt:   nullInt64(room.StartedAt),
		ClosedAt:    nullInt64(room.ClosedAt),
		ArchivedAt:  nullInt64(room.ArchivedAt),
	}, nil
}

// UpdateRoom applies a partial rename/model change.
func (c *Core) UpdateRoom(ctx context.Context, id string, name *string, model *string) (dbgen.Room, error) {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	nextName := room.Name
	if name != nil {
		nextName = sql.NullString{String: *name, Valid: *name != ""}
	}
	nextModel := room.Model
	if model != nil {
		if strings.TrimSpace(*model) == "" {
			return dbgen.Room{}, fmt.Errorf("%w: model must not be empty", ErrBadRequest)
		}
		nextModel = *model
	}
	if err := c.q.SetRoomNameAndModel(ctx, dbgen.SetRoomNameAndModelParams{
		Name:  nextName,
		Model: nextModel,
		ID:    id,
	}); err != nil {
		return dbgen.Room{}, err
	}
	updated, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	c.emitRoomUpdate(ctx, updated)
	return updated, nil
}

// Start opens a room for prompting. Only legal from `open`.
func (c *Core) Start(ctx context.Context, id string) (dbgen.Room, error) {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	if room.State != RoomOpen {
		return dbgen.Room{}, fmt.Errorf("%w: cannot start a room in state %q", ErrConflict, room.State)
	}
	if err := c.q.StartRoom(ctx, dbgen.StartRoomParams{
		StartedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:        id,
	}); err != nil {
		return dbgen.Room{}, err
	}
	updated, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	c.emitRoomUpdate(ctx, updated)
	c.emitRoomState(id, RoomStarted)
	if _, err := c.InsertMessage(ctx, id, nil, KindSystem, MsgRoomStarted); err != nil {
		c.log.Warn("core: persist start notice")
	}
	if p := c.previewsOrNil(); p != nil {
		ids, err := c.q.ListUsersByRoomForBulk(ctx, id)
		if err == nil {
			for _, uid := range ids {
				p.Trigger(id, uid)
			}
		}
	}
	c.forEachUser(ctx, id, func(uid int64) {
		c.emitAgentState(ctx, id, uid)
	})
	return updated, nil
}

// Close stops editing. Only legal from `started`; every user gets the notice.
func (c *Core) Close(ctx context.Context, id string) (dbgen.Room, error) {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	if room.State != RoomStarted {
		return dbgen.Room{}, fmt.Errorf("%w: cannot close a room in state %q", ErrConflict, room.State)
	}
	if err := c.q.CloseRoom(ctx, dbgen.CloseRoomParams{
		ClosedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:       id,
	}); err != nil {
		return dbgen.Room{}, err
	}
	updated, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	if _, err := c.InsertMessage(ctx, id, nil, KindSystem, MsgRoomClosed); err != nil {
		c.log.Warn("core: persist close notice")
	}
	c.stopRoomUsers(ctx, id)
	c.emitRoomState(id, RoomClosed)
	c.emitRoomUpdate(ctx, updated)
	return updated, nil
}

// Reopen resumes a `closed` (or `archived`) room.
func (c *Core) Reopen(ctx context.Context, id string) (dbgen.Room, error) {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	switch room.State {
	case RoomClosed:
		if err := c.q.ReopenRoom(ctx, id); err != nil {
			return dbgen.Room{}, err
		}
	case RoomArchived:
		if err := c.q.UnarchiveRoom(ctx, id); err != nil {
			return dbgen.Room{}, err
		}
		if err := c.q.ReopenRoom(ctx, id); err != nil {
			return dbgen.Room{}, err
		}
	default:
		return dbgen.Room{}, fmt.Errorf("%w: cannot reopen a room in state %q", ErrConflict, room.State)
	}
	updated, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	c.emitRoomUpdate(ctx, updated)
	c.emitRoomState(id, RoomStarted)
	if _, err := c.InsertMessage(ctx, id, nil, KindSystem, MsgRoomReopened); err != nil {
		c.log.Warn("core: persist reopen notice")
	}
	c.forEachUser(ctx, id, func(uid int64) {
		c.emitAgentState(ctx, id, uid)
	})
	return updated, nil
}

// Archive retires a `closed` room; its users are ejected by the client.
func (c *Core) Archive(ctx context.Context, id string) (dbgen.Room, error) {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	if room.State != RoomClosed {
		return dbgen.Room{}, fmt.Errorf("%w: cannot archive a room in state %q", ErrConflict, room.State)
	}
	if err := c.q.ArchiveRoom(ctx, dbgen.ArchiveRoomParams{
		ArchivedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:         id,
	}); err != nil {
		return dbgen.Room{}, err
	}
	updated, err := c.GetRoom(ctx, id)
	if err != nil {
		return dbgen.Room{}, err
	}
	c.stopRoomUsers(ctx, id)
	c.emitRoomState(id, RoomArchived)
	c.emitRoomUpdate(ctx, updated)
	return updated, nil
}

// Delete removes an `archived` room together with its users, subdirectories,
// templates, messages and history.
func (c *Core) Delete(ctx context.Context, id string) error {
	room, err := c.GetRoom(ctx, id)
	if err != nil {
		return err
	}
	if room.State != RoomArchived {
		return fmt.Errorf("%w: only an archived room can be deleted", ErrConflict)
	}
	c.stopRoomUsers(ctx, id)
	if err := c.q.DeleteRoom(ctx, id); err != nil {
		return err
	}
	if err := c.files.RemoveRoomDirs(id); err != nil {
		c.log.Warn("core: remove room directories", zap.Error(err))
	}
	return nil
}

// stopRoomUsers cancels every user's run, closes their sockets and forgets
// their live state.
func (c *Core) stopRoomUsers(ctx context.Context, roomID string) {
	c.forEachUser(ctx, roomID, func(uid int64) {
		if a := c.agentsOrNil(); a != nil {
			a.StopUser(uid)
		}
		c.hub.CloseUser(uid)
		c.reg.Forget(uid)
		if p := c.previewsOrNil(); p != nil {
			p.Drop(uid)
		}
	})
}

func (c *Core) forEachUser(ctx context.Context, roomID string, fn func(userID int64)) {
	ids, err := c.q.ListUsersByRoomForBulk(ctx, roomID)
	if err != nil {
		return
	}
	for _, id := range ids {
		fn(id)
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
