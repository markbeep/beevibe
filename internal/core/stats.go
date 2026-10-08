package core

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mark/beevibe/internal/db/gen"
)

// StatsFor returns the per-user derived values shown in the user and admin UIs.
func (c *Core) StatsFor(ctx context.Context, roomID string, userID int64) (StatsJSON, error) {
	user, err := c.GetUserInRoom(ctx, roomID, userID)
	if err != nil {
		return StatsJSON{}, err
	}
	loc, err := c.files.LOC(roomID, userID)
	if err != nil {
		return StatsJSON{}, err
	}
	history, err := c.q.ListAgentMessages(ctx, userID)
	if err != nil {
		return StatsJSON{}, err
	}
	return StatsJSON{
		TokensUsed:           user.TokensUsed,
		TokenLimit:           nullInt64(user.TokenLimit),
		LOC:                  loc,
		SitePath:             fmt.Sprintf("/rooms/%s/%d/", roomID, userID),
		AgentContextMessages: len(history),
	}, nil
}

// TouchUserLastSeen records the user's most recent authenticated request.
func (c *Core) TouchUserLastSeen(ctx context.Context, userID int64) error {
	return c.q.TouchUserLastSeen(ctx, dbgen.TouchUserLastSeenParams{
		LastSeenAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID:         userID,
	})
}
