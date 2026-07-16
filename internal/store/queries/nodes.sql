-- name: CreateNode :one
INSERT INTO nodes (id, workspace_id, seq, key, parent_id, title, description, status_id, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetNodeByID :one
SELECT * FROM nodes WHERE id = ?;

-- name: GetNodeByKey :one
SELECT * FROM nodes WHERE key = ?;

-- Flat list of a workspace's nodes (ordered by seq). parent/status filters are
-- applied in-memory by the service layer (v0 scale; keeps SQL portable, no
-- engine-specific dynamic SQL).
-- name: ListNodesByWorkspace :many
SELECT * FROM nodes WHERE workspace_id = ? ORDER BY seq;

-- name: ListRootNodes :many
SELECT * FROM nodes WHERE workspace_id = ? AND parent_id IS NULL ORDER BY seq;

-- name: ListChildren :many
SELECT * FROM nodes WHERE parent_id = ? ORDER BY seq;

-- Rollup input: status categories of a node's DIRECT children (the derived
-- aggregate itself is computed by the service layer).
-- name: ListChildStatusCategories :many
SELECT s.category AS category, COUNT(*) AS cnt
FROM nodes n
LEFT JOIN statuses s ON s.id = n.status_id
WHERE n.parent_id = ?
GROUP BY s.category;

-- name: UpdateNode :one
UPDATE nodes
SET title = ?, description = ?, status_id = ?, priority = ?, parent_id = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;

-- name: CountChildren :one
SELECT COUNT(*) FROM nodes WHERE parent_id = ?;
