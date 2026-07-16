-- name: CreateWorkspace :one
INSERT INTO workspaces (id, key, name, description, node_seq, created_at, updated_at)
VALUES (?, ?, ?, ?, 0, ?, ?)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = ?;

-- name: GetWorkspaceByKey :one
SELECT * FROM workspaces WHERE key = ?;

-- name: ListWorkspaces :many
SELECT * FROM workspaces ORDER BY created_at, key;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET name = ?, description = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- Transactional per-workspace node.seq counter (monotonic, never decreases).
-- RETURNING is supported by both modernc SQLite and pgx/PG. Called inside a Tx
-- together with the nodes INSERT so key allocation is atomic.
-- name: BumpWorkspaceNodeSeq :one
UPDATE workspaces
SET node_seq = node_seq + 1, updated_at = ?
WHERE id = ?
RETURNING node_seq;
