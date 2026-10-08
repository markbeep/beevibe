package core

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"time"

	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/ids"
)

// CreateUser allocates a token, seeds the subdirectory and queues the first
// preview (subject to MAX_USERS_PER_ROOM).
func (c *Core) CreateUser(ctx context.Context, roomID, name string, tokenLimit *int64) (UserJSON, error) {
	if _, err := c.GetRoom(ctx, roomID); err != nil {
		return UserJSON{}, err
	}
	count, err := c.q.CountUsersByRoom(ctx, roomID)
	if err != nil {
		return UserJSON{}, err
	}
	if count >= int64(c.cfg.MaxUsersPerRoom) {
		return UserJSON{}, fmt.Errorf("%w: user limit reached (%d)", ErrConflict, c.cfg.MaxUsersPerRoom)
	}
	var tl sql.NullInt64
	if tokenLimit != nil {
		tl = sql.NullInt64{Int64: *tokenLimit, Valid: true}
	}
	now := time.Now().Unix()
	var lastErr error
	for range 10 {
		user, err := c.q.CreateUser(ctx, dbgen.CreateUserParams{
			RoomID:     roomID,
			Name:       name,
			Token:      ids.UserToken(),
			CreatedAt:  now,
			TokenLimit: tl,
		})
		if err != nil {
			if isUniqueViolation(err) {
				lastErr = err
				continue
			}
			return UserJSON{}, err
		}
		if err := c.files.Seed(ctx, roomID, user.ID); err != nil {
			// A user without a subdirectory is unusable; roll the row back.
			if delErr := c.q.DeleteUser(ctx, user.ID); delErr != nil {
				c.log.Error("core: roll back user after seed failure")
			}
			return UserJSON{}, err
		}
		if p := c.previewsOrNil(); p != nil {
			p.Trigger(roomID, user.ID)
		}
		c.EmitUserUpdate(ctx, roomID, user.ID)
		return c.UserView(user), nil
	}
	return UserJSON{}, fmt.Errorf("%w: could not allocate a user token: %v", ErrConflict, lastErr)
}

// ListUsers returns every user of a room with live state merged in.
func (c *Core) ListUsers(ctx context.Context, roomID string) ([]UserJSON, error) {
	if _, err := c.GetRoom(ctx, roomID); err != nil {
		return nil, err
	}
	users, err := c.q.ListUsersByRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	out := make([]UserJSON, 0, len(users))
	for _, u := range users {
		out = append(out, c.UserView(u))
	}
	return out, nil
}

// GetUserInRoom returns one user that must belong to the room.
func (c *Core) GetUserInRoom(ctx context.Context, roomID string, userID int64) (dbgen.User, error) {
	user, err := c.q.GetUser(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return dbgen.User{}, fmt.Errorf("%w: unknown user %d", ErrNotFound, userID)
	}
	if err != nil {
		return dbgen.User{}, err
	}
	if user.RoomID != roomID {
		return dbgen.User{}, fmt.Errorf("%w: user %d is not in room %q", ErrNotFound, userID, roomID)
	}
	return user, nil
}

// SetUserTokenLimit updates the cumulative limit. Setting it to null means
// "unlimited" and also resets the consumed counter, which is the spec's
// "or resets usage" path.
func (c *Core) SetUserTokenLimit(ctx context.Context, roomID string, userID int64, limit *int64) (UserJSON, error) {
	if _, err := c.GetUserInRoom(ctx, roomID, userID); err != nil {
		return UserJSON{}, err
	}
	if limit == nil {
		if err := c.q.SetUserTokenLimit(ctx, dbgen.SetUserTokenLimitParams{ID: userID}); err != nil {
			return UserJSON{}, err
		}
		if err := c.q.ResetUserTokensUsed(ctx, userID); err != nil {
			return UserJSON{}, err
		}
	} else {
		if *limit < 0 {
			return UserJSON{}, fmt.Errorf("%w: token limit must not be negative", ErrBadRequest)
		}
		if err := c.q.SetUserTokenLimit(ctx, dbgen.SetUserTokenLimitParams{
			TokenLimit: sql.NullInt64{Int64: *limit, Valid: true},
			ID:         userID,
		}); err != nil {
			return UserJSON{}, err
		}
	}
	user, err := c.q.GetUser(ctx, userID)
	if err != nil {
		return UserJSON{}, err
	}
	c.EmitUserUpdate(ctx, roomID, userID)
	c.emitAgentState(ctx, roomID, userID)
	return c.UserView(user), nil
}

// KickUser cancels the user's runs and disconnects them; the token stays valid.
func (c *Core) KickUser(ctx context.Context, roomID string, userID int64) error {
	if _, err := c.GetUserInRoom(ctx, roomID, userID); err != nil {
		return err
	}
	if a := c.agentsOrNil(); a != nil {
		a.StopUser(userID)
	}
	if err := c.q.MarkUserKicked(ctx, dbgen.MarkUserKickedParams{
		KickedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:       userID,
	}); err != nil {
		return err
	}
	c.hub.CloseUser(userID)
	c.reg.Forget(userID)
	c.EmitUserUpdate(ctx, roomID, userID)
	return nil
}

// DeleteUser cancels runs, removes the user, their subdirectory and history.
func (c *Core) DeleteUser(ctx context.Context, roomID string, userID int64) error {
	if _, err := c.GetUserInRoom(ctx, roomID, userID); err != nil {
		return err
	}
	if a := c.agentsOrNil(); a != nil {
		a.StopUser(userID)
	}
	c.hub.CloseUser(userID)
	if err := c.q.DeleteUser(ctx, userID); err != nil {
		return err
	}
	if err := c.files.RemoveUserDir(roomID, userID); err != nil {
		c.log.Warn("core: remove user directory")
	}
	if p := c.previewsOrNil(); p != nil {
		p.Drop(userID)
	}
	c.reg.Forget(userID)
	return nil
}

// CancelUser aborts the user's in-flight run; a conflict when nothing is in
// flight (the API returns 409).
func (c *Core) CancelUser(ctx context.Context, roomID string, userID int64) error {
	if _, err := c.GetUserInRoom(ctx, roomID, userID); err != nil {
		return err
	}
	a := c.agentsOrNil()
	if a == nil || !a.InFlight(userID) {
		return fmt.Errorf("%w: nothing in flight", ErrConflict)
	}
	if err := a.Cancel(userID); err != nil {
		if errors.Is(err, ErrConflict) {
			return fmt.Errorf("%w: nothing in flight", ErrConflict)
		}
		return err
	}
	c.EmitUserUpdate(ctx, roomID, userID)
	return nil
}

// BulkRequest is the decoded POST /users/bulk body.
type BulkRequest struct {
	Action string
	// All means "every user in the room"; otherwise only UserIDs are affected.
	All        bool
	UserIDs    []int64
	Text       *string
	TokenLimit *int64
}

// Bulk applies one admin action to a set of users and returns how many were
// affected.
func (c *Core) Bulk(ctx context.Context, roomID string, req BulkRequest) (int, error) {
	targets, err := c.resolveTargets(ctx, roomID, req)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	switch req.Action {
	case "kick":
		affected := 0
		for _, uid := range targets {
			if err := c.KickUser(ctx, roomID, uid); err != nil {
				return affected, err
			}
			affected++
		}
		return affected, nil
	case "delete":
		affected := 0
		for _, uid := range targets {
			if err := c.DeleteUser(ctx, roomID, uid); err != nil {
				return affected, err
			}
			affected++
		}
		return affected, nil
	case "message":
		if req.Text == nil || *req.Text == "" {
			return 0, fmt.Errorf("%w: text is required for the message action", ErrBadRequest)
		}
		affected := 0
		for _, uid := range targets {
			if _, err := c.MessageUser(ctx, roomID, uid, *req.Text); err != nil {
				return affected, err
			}
			affected++
		}
		return affected, nil
	case "set-token-limit":
		affected := 0
		for _, uid := range targets {
			if _, err := c.SetUserTokenLimit(ctx, roomID, uid, req.TokenLimit); err != nil {
				return affected, err
			}
			affected++
		}
		return affected, nil
	case "reset":
		c.ResetAsync(roomID, targets, ResetScopeBulk)
		return len(targets), nil
	default:
		return 0, fmt.Errorf("%w: unknown bulk action %q", ErrBadRequest, req.Action)
	}
}

func (c *Core) resolveTargets(ctx context.Context, roomID string, req BulkRequest) ([]int64, error) {
	roomUsers, err := c.q.ListUsersByRoomForBulk(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if req.All {
		return roomUsers, nil
	}
	if len(req.UserIDs) == 0 {
		return nil, nil
	}
	inRoom := make(map[int64]bool, len(roomUsers))
	for _, id := range roomUsers {
		inRoom[id] = true
	}
	out := make([]int64, 0, len(req.UserIDs))
	for _, id := range req.UserIDs {
		if inRoom[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// ResetAsync re-seeds the given users in the background, reporting progress to
// the admin channel (the HTTP routes answer 202).
func (c *Core) ResetAsync(roomID string, userIDs []int64, scope string) {
	go c.ResetUsers(context.Background(), roomID, userIDs, scope)
}

// ResetUsers wipes the users' subdirectories and agent history, re-seeds them
// from the room template and reports progress. It is synchronous so it can be
// driven from tests; ResetAsync wraps it for the HTTP routes.
func (c *Core) ResetUsers(ctx context.Context, roomID string, userIDs []int64, scope string) {
	total := len(userIDs)
	c.hub.ToAdmins(frameResetProgress{Type: "reset.progress", Scope: scope, Total: total, Running: true})
	for done, uid := range userIDs {
		c.resetOneUser(ctx, roomID, uid)
		c.hub.ToAdmins(frameResetProgress{
			Type:    "reset.progress",
			Scope:   scope,
			Total:   total,
			Done:    done + 1,
			Running: done+1 < total,
		})
	}
}

func (c *Core) resetOneUser(ctx context.Context, roomID string, userID int64) {
	if a := c.agentsOrNil(); a != nil {
		a.StopUser(userID)
	}
	c.reg.SetResetting(userID, true)
	c.emitAgentState(ctx, roomID, userID)
	if p := c.previewsOrNil(); p != nil {
		p.Drop(userID)
	}
	if _, err := c.InsertMessage(ctx, roomID, &userID, KindSystem, MsgResetStart); err != nil {
		c.log.Warn("core: persist reset start notice")
	}
	if err := c.files.Seed(ctx, roomID, userID); err != nil {
		c.log.Error("core: seed user directory")
	}
	if err := c.q.DeleteAgentMessagesByUser(ctx, userID); err != nil {
		c.log.Error("core: delete agent history")
	}
	if _, err := c.InsertMessage(ctx, roomID, &userID, KindSystem, MsgResetDone); err != nil {
		c.log.Warn("core: persist reset done notice")
	}
	c.reg.SetResetting(userID, false)
	if p := c.previewsOrNil(); p != nil {
		p.Trigger(roomID, userID)
	}
	c.emitAgentState(ctx, roomID, userID)
	c.EmitUserUpdate(ctx, roomID, userID)
}

// ReseedForTemplate re-seeds every user of a room after a template upload and
// reports progress with the `template` scope (D2).
func (c *Core) ReseedForTemplate(ctx context.Context, roomID string, userIDs []int64) {
	c.ResetUsers(ctx, roomID, userIDs, ResetScopeTemplate)
}

// UserIDs lists every user of a room.
func (c *Core) UserIDs(ctx context.Context, roomID string) ([]int64, error) {
	return c.q.ListUsersByRoomForBulk(ctx, roomID)
}

// ExportCSV renders the room's tokens as `name,token` rows (AREDIT-2).
func (c *Core) ExportCSV(ctx context.Context, roomID string) ([]byte, error) {
	if _, err := c.GetRoom(ctx, roomID); err != nil {
		return nil, err
	}
	users, err := c.q.ListUsersByRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"name", "token"}); err != nil {
		return nil, err
	}
	for _, u := range users {
		if err := w.Write([]string{u.Name, u.Token}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
