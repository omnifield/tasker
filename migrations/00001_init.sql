-- Канон-схема tasker: «дерево разработки» (рекурсивный узел + workspace + статус-категории
-- + dual-id + typed relations + activity). SQL — совместимый SQLite↔Postgres (канон go.md):
-- только TEXT/INTEGER, ноль AUTOINCREMENT/SERIAL (id/seq генерит app-слой), ноль engine-специфики.
-- FK ОБЪЯВЛЕНЫ (документируют инвариант + PRAGMA foreign_keys для SQLite), но enforce'им ещё и
-- сами в сервис-слое (Jira-урок: не полагаться на логические FK). Таймстемпы — TEXT RFC3339
-- (портируется 1:1, у SQLite нет нативного timestamp-типа).

-- +goose Up
-- +goose StatementBegin
CREATE TABLE workspaces (
  id          TEXT PRIMARY KEY,
  key         TEXT NOT NULL UNIQUE,          -- короткий человекочитаемый префикс, напр. "TASK"
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  node_seq    INTEGER NOT NULL DEFAULT 0,    -- per-workspace счётчик node.seq (транзакционный, не убывает)
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Статус per-workspace: фикс-КАТЕГОРИЯ (backlog|todo|in_progress|done|canceled) — заложена полем
-- с 1-го дня (даже если старт бинарный) — + кастомные name/color. Роллап читает именно category.
CREATE TABLE statuses (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  category     TEXT NOT NULL,               -- backlog|todo|in_progress|done|canceled
  name         TEXT NOT NULL,
  color        TEXT NOT NULL DEFAULT '',
  position     INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_statuses_ws ON statuses(workspace_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Node — рекурсивный core (type-less Issue-канон). Дерево через parent_id (primary-структура),
-- НЕ через relation. key = "<WS>-<seq>" СТАБИЛЬНЫЙ/immutable (по нему рефают из chater).
CREATE TABLE nodes (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  seq          INTEGER NOT NULL,            -- per-workspace порядковый (unique с workspace_id)
  key          TEXT NOT NULL,               -- "<WS>-<seq>", immutable
  parent_id    TEXT REFERENCES nodes(id),   -- nullable self-FK → рекурсия/дерево
  title        TEXT NOT NULL,
  description  TEXT NOT NULL DEFAULT '',
  status_id    TEXT REFERENCES statuses(id),
  priority     INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE (workspace_id, seq)
);
-- +goose StatementEnd
-- +goose StatementBegin
-- key глобально уникален (WS-префиксы уникальны → коллизий нет) → dual-id резолв по key
-- однозначен без workspace-контекста (chater рефает "TASK-123" напрямую).
CREATE UNIQUE INDEX idx_nodes_key ON nodes(key);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_nodes_ws ON nodes(workspace_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_nodes_parent ON nodes(parent_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_nodes_status ON nodes(status_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Relation — typed first-class links (blocks|relates|duplicate). cross-workspace допустим.
CREATE TABLE relations (
  id         TEXT PRIMARY KEY,
  from_node  TEXT NOT NULL REFERENCES nodes(id),
  to_node    TEXT NOT NULL REFERENCES nodes(id),
  kind       TEXT NOT NULL,                 -- blocks|relates|duplicate
  created_at TEXT NOT NULL,
  UNIQUE (from_node, to_node, kind)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_relations_from ON relations(from_node);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_relations_to ON relations(to_node);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE labels (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  name         TEXT NOT NULL,
  color        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  UNIQUE (workspace_id, name)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE node_labels (
  node_id  TEXT NOT NULL REFERENCES nodes(id),
  label_id TEXT NOT NULL REFERENCES labels(id),
  PRIMARY KEY (node_id, label_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Assignee — актор (агент/человек handle). Минимально; может ссылаться на identity-мост позже.
CREATE TABLE node_assignees (
  node_id    TEXT NOT NULL REFERENCES nodes(id),
  actor      TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (node_id, actor)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Activity — typed timeline с 1-го дня. data = JSON-blob (payload события). comments — kind='commented'.
CREATE TABLE activity (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id),
  actor      TEXT NOT NULL,
  kind       TEXT NOT NULL,                 -- created|status_changed|assigned|relation_added|commented|...
  data       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_activity_node ON activity(node_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS activity;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS node_assignees;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS node_labels;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS labels;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS relations;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS nodes;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS statuses;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS workspaces;
-- +goose StatementEnd
