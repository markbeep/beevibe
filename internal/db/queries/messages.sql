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

-- name: ListChatForUserPage :many
-- One page of a user's chat: messages addressed to that user plus room-wide
-- broadcasts. `before` is an exclusive id cursor; NULL means "the newest page".
-- The page is returned oldest-first.
SELECT * FROM (
    SELECT * FROM messages
    WHERE (user_id = sqlc.arg(user_id) OR (room_id = sqlc.arg(room_id) AND user_id IS NULL))
      AND id < COALESCE(sqlc.narg(before), 9223372036854775807)
    ORDER BY id DESC
    LIMIT sqlc.arg(page_limit)
) ORDER BY id;

-- name: DeleteMessagesByUser :exec
DELETE FROM messages WHERE user_id = ?;
