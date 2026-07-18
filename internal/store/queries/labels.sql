-- name: CreateLabel :one
INSERT INTO labels (id, workspace_id, name, color, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLabel :one
SELECT * FROM labels WHERE id = ?;

-- name: ListLabelsByWorkspace :many
SELECT * FROM labels WHERE workspace_id = ? ORDER BY name;

-- name: AddNodeLabel :exec
INSERT INTO node_labels (node_id, label_id) VALUES (?, ?)
ON CONFLICT (node_id, label_id) DO NOTHING;

-- name: RemoveNodeLabel :exec
DELETE FROM node_labels WHERE node_id = ? AND label_id = ?;

-- name: DeleteNodeLabelsForNode :exec
DELETE FROM node_labels WHERE node_id = ?;

-- name: ListNodeLabels :many
SELECT l.* FROM labels l
JOIN node_labels nl ON nl.label_id = l.id
WHERE nl.node_id = ?
ORDER BY l.name;
