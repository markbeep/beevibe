-- name: InsertMessage :one
INSERT INTO messages (room_id, user_id, kind, text, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMessage :one
SELECT * FROM messages WHERE id = ?;

-- name: ListMessagesByUser :many
SELECT * FROM messages WHERE user_id = ? ORDER BY id;

-- name: ListMessagesByRoom :many
SELECT * FROM messages WHERE room_id = ? ORDER BY id;

-- name: ListChatForUser :many
-- the user's own chat: everything addressed to them plus room-wide broadcasts
SELECT * FROM messages
WHERE user_id = ? OR (room_id = ? AND user_id IS NULL)
ORDER BY id;

-- name: ListRecentChatForUser :many
SELECT * FROM (
    SELECT * FROM messages
    WHERE user_id = ? OR (room_id = ? AND user_id IS NULL)
    ORDER BY id DESC
    LIMIT ?
) ORDER BY id;

-- name: DeleteMessagesByUser :exec
DELETE FROM messages WHERE user_id = ?;
