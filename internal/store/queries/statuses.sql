-- name: CreateStatus :one
INSERT INTO statuses (id, workspace_id, category, name, color, position, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetStatus :one
SELECT * FROM statuses WHERE id = ?;

-- name: ListStatusesByWorkspace :many
SELECT * FROM statuses WHERE workspace_id = ? ORDER BY position, created_at;
