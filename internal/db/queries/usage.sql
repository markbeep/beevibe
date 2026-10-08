-- name: InsertUsageEvent :one
INSERT INTO usage_events (run_id, user_id, input_tokens, output_tokens, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListUsageEventsByUser :many
SELECT * FROM usage_events WHERE user_id = ? ORDER BY id;

-- name: ListUsageEventsByRun :many
SELECT * FROM usage_events WHERE run_id = ? ORDER BY id;

-- name: SumUsageByUser :one
SELECT
    COALESCE(SUM(input_tokens), 0)  AS input_tokens,
    COALESCE(SUM(output_tokens), 0) AS output_tokens
FROM usage_events WHERE user_id = ?;

-- name: SumUsageByRoom :one
SELECT
    COALESCE(SUM(u.input_tokens), 0)  AS input_tokens,
    COALESCE(SUM(u.output_tokens), 0) AS output_tokens
FROM usage_events u
JOIN users usr ON usr.id = u.user_id
WHERE usr.room_id = ?;
