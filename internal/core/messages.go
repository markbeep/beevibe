package core

import (
	"context"
	"database/sql"
	"time"

	"github.com/mark/beevibe/internal/db/gen"
)

// DefaultChatPage is the page size used when a caller does not specify one.
const DefaultChatPage = 50

// InsertMessage persists one chat entry, then fans it out: to the target user's
// sockets (or every user of the room for a room-wide entry) and to every admin
// socket (D3).
func (c *Core) InsertMessage(ctx context.Context, roomID string, userID *int64, kind, text string) (MessageJSON, error) {
	var uid sql.NullInt64
	if userID != nil {
		uid = sql.NullInt64{Int64: *userID, Valid: true}
	}
	row, err := c.q.InsertMessage(ctx, dbgen.InsertMessageParams{
		RoomID:    roomID,
		UserID:    uid,
		Kind:      kind,
		Text:      text,
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return MessageJSON{}, err
	}
	msg := messageJSON(row)
	c.pushMessage(ctx, row, msg)
	return msg, nil
}

func (c *Core) pushMessage(ctx context.Context, row dbgen.Message, msg MessageJSON) {
	frame := frameChatAppend{Type: "chat.append", Message: msg}
	if row.UserID.Valid {
		c.hub.ToUser(row.UserID.Int64, frame)
	} else {
		ids, err := c.q.ListUsersByRoomForBulk(ctx, row.RoomID)
		if err == nil {
			for _, id := range ids {
				c.hub.ToUser(id, frame)
			}
		}
	}
	c.hub.ToAdmins(frame)
}

// Broadcast persists a room-wide admin message (Q-UI-15).
func (c *Core) Broadcast(ctx context.Context, roomID, text string) (MessageJSON, error) {
	return c.InsertMessage(ctx, roomID, nil, KindAdmin, text)
}

// MessageUser persists an admin message addressed to one user and clears their
// raise-hand (D4).
func (c *Core) MessageUser(ctx context.Context, roomID string, userID int64, text string) (MessageJSON, error) {
	msg, err := c.InsertMessage(ctx, roomID, &userID, KindAdmin, text)
	if err != nil {
		return MessageJSON{}, err
	}
	if c.reg.HelpPending(userID) {
		c.reg.SetHelp(userID, false)
		c.EmitUserUpdate(ctx, roomID, userID)
	}
	return msg, nil
}

// SetHelp raises or clears the user's raise-hand (the Help button is a toggle).
// Raising posts the help chat entry once; clearing posts nothing, because the
// admin already saw the badge and its disappearance is the signal. The admin can
// clear it too, from the grid drawer or the room's user list.
func (c *Core) SetHelp(ctx context.Context, roomID string, userID int64, on bool) error {
	if on == c.reg.HelpPending(userID) {
		return nil // idempotent: never post the entry twice for one raise
	}
	if on {
		if _, err := c.InsertMessage(ctx, roomID, &userID, KindHelp, MsgHelpRequest); err != nil {
			return err
		}
	}
	c.reg.SetHelp(userID, on)
	c.EmitUserUpdate(ctx, roomID, userID)
	return nil
}

// ListUserChat returns one page of a user's chat: entries addressed to them
// plus room-wide broadcasts, oldest-first within the page. A before of 0 means
// "the newest page".
func (c *Core) ListUserChat(ctx context.Context, roomID string, userID int64, limit int, before int64) ([]MessageJSON, error) {
	if limit <= 0 {
		limit = DefaultChatPage
	}
	if limit > 500 {
		limit = 500
	}
	var cursor sql.NullInt64
	if before > 0 {
		cursor = sql.NullInt64{Int64: before, Valid: true}
	}
	rows, err := c.q.ListChatForUserPage(ctx, dbgen.ListChatForUserPageParams{
		UserID:    sql.NullInt64{Int64: userID, Valid: true},
		RoomID:    roomID,
		Before:    cursor,
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]MessageJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, messageJSON(row))
	}
	return out, nil
}

func messageJSON(m dbgen.Message) MessageJSON {
	return MessageJSON{ID: m.ID, Kind: m.Kind, Text: m.Text, At: m.CreatedAt}
}
