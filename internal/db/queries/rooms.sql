-- name: CreateRoom :one
INSERT INTO rooms (id, name, model, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetRoom :one
SELECT * FROM rooms WHERE id = ?;

-- name: ListRooms :many
SELECT * FROM rooms ORDER BY created_at DESC;

-- name: CountRooms :one
SELECT COUNT(*) AS count FROM rooms;

-- name: RenameRoom :exec
UPDATE rooms SET name = ? WHERE id = ?;

-- name: SetRoomNameAndModel :exec
UPDATE rooms SET name = ?, model = ? WHERE id = ?;

-- name: StartRoom :exec
UPDATE rooms SET state = 'started', started_at = ? WHERE id = ?;

-- name: ReopenRoom :exec
UPDATE rooms SET state = 'started', closed_at = NULL WHERE id = ?;

-- name: CloseRoom :exec
UPDATE rooms SET state = 'closed', closed_at = ? WHERE id = ?;

-- name: ArchiveRoom :exec
UPDATE rooms SET state = 'archived', archived_at = ? WHERE id = ?;

-- name: UnarchiveRoom :exec
UPDATE rooms SET state = 'closed', archived_at = NULL WHERE id = ?;

-- name: DeleteRoom :exec
DELETE FROM rooms WHERE id = ?;
