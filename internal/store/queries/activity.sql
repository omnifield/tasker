-- name: CreateActivity :one
INSERT INTO activity (id, node_id, actor, kind, data, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListActivityForNode :many
SELECT * FROM activity WHERE node_id = ? ORDER BY created_at, id;
