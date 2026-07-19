-- Cross-product proposals: a node can arrive in a FOREIGN workspace as a quarantined
-- proposal (origin='proposal'), excluded from the roadmap roll-up until an explicit accept
-- promotes it (origin->'native' + a real parent). Agent-agnostic: tasker enforces only the
-- STRUCTURAL gate (nothing enters the roadmap without accept) and records who proposed it;
-- WHO may accept is a consumer/identity concern, not enforced here.
-- SQLite<->Postgres compatible: TEXT columns, ADD COLUMN with defaults (backfills existing
-- rows to 'native'), no engine specifics.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE nodes ADD COLUMN origin TEXT NOT NULL DEFAULT 'native';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE nodes ADD COLUMN proposed_by TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE nodes ADD COLUMN source_ws TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
-- Inbox lookup: proposals of a workspace. Also serves the roadmap filter (origin='native').
CREATE INDEX idx_nodes_origin ON nodes(workspace_id, origin);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_nodes_origin;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE nodes DROP COLUMN source_ws;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE nodes DROP COLUMN proposed_by;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE nodes DROP COLUMN origin;
-- +goose StatementEnd
