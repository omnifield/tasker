-- name: AddNodeAssignee :exec
INSERT INTO node_assignees (node_id, actor, created_at) VALUES (?, ?, ?)
ON CONFLICT (node_id, actor) DO NOTHING;

-- name: RemoveNodeAssignee :exec
DELETE FROM node_assignees WHERE node_id = ? AND actor = ?;

-- name: ListNodeAssignees :many
SELECT actor FROM node_assignees WHERE node_id = ? ORDER BY created_at;
