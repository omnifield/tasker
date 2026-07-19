-- name: CreateNode :one
INSERT INTO nodes (id, workspace_id, seq, key, parent_id, title, description, status_id, priority, origin, proposed_by, source_ws, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetNodeByID :one
SELECT * FROM nodes WHERE id = ?;

-- name: GetNodeByKey :one
SELECT * FROM nodes WHERE key = ?;

-- Flat list of a workspace's roadmap nodes. Ordered by priority (lower = higher priority,
-- P0 first), then seq for stable tie-break. parent/status filters are applied in-memory by
-- the service layer (v0 scale; keeps SQL portable, no dynamic SQL). Proposals (origin!=native)
-- live only in the inbox, never the roadmap.
-- name: ListNodesByWorkspace :many
SELECT * FROM nodes WHERE workspace_id = ? AND origin = 'native' ORDER BY priority, seq;

-- Roadmap roots (tree top level). Excludes un-accepted proposals (origin='proposal'): they are
-- parent-less like roots but must not surface in the roadmap until accepted.
-- name: ListRootNodes :many
SELECT * FROM nodes WHERE workspace_id = ? AND parent_id IS NULL AND origin = 'native' ORDER BY priority, seq;

-- Inbox: pending cross-product proposals of a workspace, awaiting accept/decline.
-- name: ListInboxNodes :many
SELECT * FROM nodes WHERE workspace_id = ? AND origin = 'proposal' ORDER BY priority, seq;

-- name: ListChildren :many
SELECT * FROM nodes WHERE parent_id = ? ORDER BY priority, seq;

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

-- Accept a proposal into the roadmap: flip origin to native and set the parent/status the
-- receiving architect chose. Stable key is preserved (inbound references stay valid).
-- name: AcceptProposalNode :one
UPDATE nodes SET parent_id = ?, status_id = ?, origin = 'native', updated_at = ? WHERE id = ?
RETURNING *;

-- Decline a proposal: set the workspace's canceled status; origin stays 'proposal' so it stays
-- in inbox history, out of the roadmap.
-- name: DeclineProposalNode :one
UPDATE nodes SET status_id = ?, updated_at = ? WHERE id = ?
RETURNING *;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;

-- name: CountChildren :one
SELECT COUNT(*) FROM nodes WHERE parent_id = ?;
