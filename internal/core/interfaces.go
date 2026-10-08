package core

import (
	"context"
	"errors"
)

// Sentinels the HTTP layer maps onto the frozen error codes.
var (
	// ErrNotFound maps to 404 not_found.
	ErrNotFound = errors.New("not found")
	// ErrConflict maps to 409 conflict.
	ErrConflict = errors.New("conflict")
	// ErrBadRequest maps to 400 bad_request.
	ErrBadRequest = errors.New("bad request")
)

// Agents is the subset of the agent supervisor the core drives. Keeping it an
// interface lets internal/core stay independent of internal/agent.
type Agents interface {
	Enqueue(ctx context.Context, roomID string, userID int64, prompt string) (runID int64, queued bool, depth int, err error)
	Cancel(userID int64) error
	State(userID int64) (state string, depth int)
	RunID(userID int64) *int64
	StopUser(userID int64)
	ResetContext(ctx context.Context, roomID string, userID int64) error
	StopAll()
	InFlight(userID int64) bool
}

// Previewer is the subset of the preview coordinator the core drives.
type Previewer interface {
	Trigger(roomID string, userID int64)
	Force(roomID string, userID int64)
	Image(userID int64) (data []byte, rev int64, ok bool)
	State(userID int64) string
	Drop(userID int64)
	Close()
}
