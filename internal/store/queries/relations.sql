-- name: CreateRelation :one
INSERT INTO relations (id, from_node, to_node, kind, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- All links where the node is source OR target (typed links visible both ways).
-- name: ListRelationsForNode :many
SELECT * FROM relations
WHERE from_node = ? OR to_node = ?
ORDER BY created_at;

-- name: DeleteRelation :exec
DELETE FROM relations WHERE id = ?;

-- All links touching the node (either side) -- for node deletion cleanup.
-- name: DeleteRelationsForNode :exec
DELETE FROM relations WHERE from_node = ? OR to_node = ?;

-- name: GetRelation :one
SELECT * FROM relations WHERE id = ?;
