package core

import (
	"context"
	"fmt"
)

// EnqueuePrompt hands a transcript to the user's agent queue.
func (c *Core) EnqueuePrompt(ctx context.Context, roomID string, userID int64, prompt string) (int64, bool, int, error) {
	a := c.agentsOrNil()
	if a == nil {
		return 0, false, 0, fmt.Errorf("%w: the agent is unavailable", ErrConflict)
	}
	return a.Enqueue(ctx, roomID, userID, prompt)
}

// ResetAgentContext cancels the in-flight run, clears the queue and deletes the
// user's conversation history (Q-AGENT-16).
func (c *Core) ResetAgentContext(ctx context.Context, roomID string, userID int64) error {
	a := c.agentsOrNil()
	if a == nil {
		return fmt.Errorf("%w: the agent is unavailable", ErrConflict)
	}
	return a.ResetContext(ctx, roomID, userID)
}

// AgentStateOf returns the user's agent state ("" when the supervisor is not
// attached).
func (c *Core) AgentStateOf(userID int64) string {
	a := c.agentsOrNil()
	if a == nil {
		return ""
	}
	state, _ := a.State(userID)
	return state
}

// QueueDepthOf returns how many prompts are waiting behind the running one.
func (c *Core) QueueDepthOf(userID int64) int {
	a := c.agentsOrNil()
	if a == nil {
		return 0
	}
	_, depth := a.State(userID)
	return depth
}

// InFlight reports whether a run is executing for the user.
func (c *Core) InFlight(userID int64) bool {
	a := c.agentsOrNil()
	return a != nil && a.InFlight(userID)
}
