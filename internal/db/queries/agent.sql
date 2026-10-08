-- name: NextAgentMessageSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 AS next_seq
FROM agent_messages WHERE user_id = ?;

-- name: AppendAgentMessage :one
INSERT INTO agent_messages (user_id, seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAgentMessages :many
SELECT * FROM agent_messages WHERE user_id = ? ORDER BY seq;

-- name: ListAgentMessagesWindow :many
-- sliding window: the most recent N turns, returned oldest-first
SELECT * FROM (
    SELECT * FROM agent_messages WHERE user_id = ? ORDER BY seq DESC LIMIT ?
) ORDER BY seq;

-- name: DeleteAgentMessagesByUser :exec
DELETE FROM agent_messages WHERE user_id = ?;

-- name: CreateAgentRun :one
INSERT INTO agent_runs (user_id, prompt, status, started_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetAgentRun :one
SELECT * FROM agent_runs WHERE id = ?;

-- name: FinishAgentRun :exec
UPDATE agent_runs
SET status = ?, finished_at = ?, steps = ?
WHERE id = ?;

-- name: ListAgentRunsByUser :many
SELECT * FROM agent_runs WHERE user_id = ? ORDER BY id DESC;

-- name: FailStaleAgentRuns :exec
-- on startup: runs left 'running' by a crash are marked failed
UPDATE agent_runs
SET status = 'failed', finished_at = ?
WHERE status IN ('queued','running');

-- name: StartAgentRun :exec
-- queued -> running transition; CreateAgentRun cannot express it and
-- FinishAgentRun also writes finished_at/steps.
UPDATE agent_runs SET status = 'running', started_at = ? WHERE id = ?;
