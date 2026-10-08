-- name: CreateUser :one
INSERT INTO users (room_id, name, token, created_at, token_limit)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByToken :one
SELECT * FROM users WHERE token = ?;

-- name: ListUsersByRoom :many
SELECT * FROM users WHERE room_id = ? ORDER BY id;

-- name: CountUsersByRoom :one
SELECT COUNT(*) AS count FROM users WHERE room_id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: SetUserTokenLimit :exec
UPDATE users SET token_limit = ? WHERE id = ?;

-- name: AddToUserTokensUsed :exec
UPDATE users SET tokens_used = tokens_used + ? WHERE id = ?;

-- name: ResetUserTokensUsed :exec
UPDATE users SET tokens_used = 0 WHERE id = ?;

-- name: TouchUserLastSeen :exec
UPDATE users SET last_seen_at = ? WHERE id = ?;

-- name: MarkUserKicked :exec
UPDATE users SET kicked_at = ? WHERE id = ?;

-- name: ListUsersByRoomForBulk :many
SELECT id FROM users WHERE room_id = ? ORDER BY id;
