# Бриф: tasker founding-backend v0 — дерево разработки (Go, REST)

> **Адресат:** архитектор / owner **tasker** (репо tasker) · tasker = flow-require-pr
> **Заказчик:** workspace-архитектор (omnifield-hub)
> **Опора:** `hub/patterns/task-manager-canon.md` (verified рыночный канон, cited) + видение
> [[tasker-founding-vision]] (рекурсивный узел + роллап + пресет-обвес). MODEL §3 (композируемость).

## Концепт (что строим)
**tasker = durable «дерево разработки» — источник правды дизайна продуктов.** Концепт→механики→**каждая
нода продумана+задокументирована**. Через год: открыл ветку → прошёлся по нодам → сориентировался, не лазя
в код/брифы. Позже: chater даёт разговорный слой (брифы = рефы на ноды tasker), UI — на своём фреймворке.
Сейчас — **бэк-фундамент: модель узла/дерева + REST API**, на готовой инфре.

## Модель (канон рынка × наше дерево) — зафиксировано
- **Node** (рекурсивный core = Issue-канон, **type-less**): `id` (UUID surrogate), `workspace_id`,
  **`key`** (`<WS>-<n>` per-workspace sequential, **СТАБИЛЬНЫЙ/immutable** — по нему рефают из chater, не
  ре-индексим при переносе), `parent_id` (FK self, nullable — **рекурсия/дерево, primary-структура**),
  `title` (required), `description`, `status_id`, `priority`, `created_at`, `updated_at`.
- **Dual-id (🔁 канон):** UUID **и** `key` резолвят один узел в API. `key` генерим **транзакционным
  per-workspace счётчиком** (unique(workspace_id, seq), не убывает).
- **Роллап (наш дифференциатор):** прогресс/статус родителя = **derived** из детей (владеет **сервис-слой**,
  не raw-колонка). Форму роллапа (доля done / rolled-up-категория) — owner фиксирует, минимально но явно.
- **Workspace** (группа-тир): `id`, `key` (короткий), `name`, `description`. Продукт ИЛИ концерн.
- **Status:** per-workspace; **фикс-категория** (`backlog|todo|in_progress|done|canceled`) + кастом
  `name/color`. Пол — можно бинарный на старте, но категория-поле заложить.
- **Relation** (🔁 typed first-class links): `from_node`, `to_node`, `kind` (`blocks|relates|duplicate`),
  **cross-workspace допустим**. (parent/child — через `parent_id`, НЕ через relation.)
- **Label** (per-workspace) + `node_labels` join (m2m). **Assignee** — актор (агент/человек), `node_assignees`
  join (минимально; может ссылаться на identity моста позже).
- **Activity** (🔁 typed timeline, с 1-го дня): `node_id`, `actor`, `kind` (`created|status_changed|
  assigned|relation_added|commented|…`), `data`, `created_at`. Comments — как activity-kind или дочерняя.

## API — REST (не GraphQL; ложится на дверь + webhooks)
- Нативный префикс `/tasker/` (дверь: `/api/tasker/…` → rewrite → `tasker:<port>/tasker/…`, как chater).
- Ресурсы: `workspaces`, `workspaces/{ws}/nodes`, `nodes/{key}` (+ `/children`, `/relations`, `/activity`),
  `labels`. CRUD + список/фильтр (по workspace/parent/status). `healthz`.
- **Одна канон-схема питает API; webhook-payload зеркалит сущность** (события `create/update/remove`) —
  заложить хук (полноценные webhooks — итерация 2, но форма событий канон с 1-го дня).
- Auth — token-stub (`Bearer <handle>`), как chater v0. Bind **0.0.0.0** (G1).

## Персистентность (Go + SQLite→Postgres)
- **sqlc** (типы из SQL) + **goose** миграции (идемпотентный `up` на старте). SQL — **совместимый
  SQLite↔Postgres** (канон go.md); **FK enforce'им сами** (Jira-урок: не полагаться на логические).
- Нормализ-ядро: центральная `nodes` + дети/relations по FK + join-таблицы (labels/assignees). Activity —
  отдельная таблица.
- БД — на **per-product волюме** (как chater `chater-data`); путь из конфига (`TASKER_DB`), не репо/`/tmp`.
- Конфиг env-only (`TASKER_PORT`, `TASKER_DB`), маленькая `Config` с явным парсом.

## Инфра-хукап (всё на готовом фундаменте)
- `omnifield.yaml`: `name: tasker`, `type: fullstack`, routes `/api/tasker`→**8030** (backend),
  `/tasker`→5173 (front, позже). Conform `registry/ports.md`.
- `devbox.services.json`: `backend` (env `TASKER_PORT=8030 TASKER_DB=<volume>` go run; health `/tasker/healthz`;
  bind 0.0.0.0). БД-волюм + `chown` в devcontainer postCreate (как chater).
- go-канон: `cmd/tasker/main.go` (graceful shutdown, slog), `internal/{config,store,httpapi}`, `migrations/`.

## MVP-граница (research §7)
- **В MVP:** Workspace + Node (рекурсив) + категор-статусы + dual-id + parent/child + typed relations
  (blocks/relates) + labels/assignees + activity-timeline + REST CRUD + роллап-derived + манифест→дверь + health.
- **Отложить:** issue-types, initiatives/cycles, SLA, time-tracking/estimates, произвольные стейт-машины,
  custom fields, полноценные webhooks-доставка, фронт/UI.

## DoD (зона tasker)
- [ ] Модель (nodes/workspaces/status/relations/labels/assignees/activity) + миграции (sqlc/goose), SQLite↔PG-совместимо, FK enforced.
- [ ] REST API `/tasker/*` (CRUD, дерево через parent_id, dual-id резолв, роллап-derived), bind 0.0.0.0, `/tasker/healthz`→200.
- [ ] `omnifield.yaml` (/api/tasker→8030) + `devbox.services.json` (backend, per-product БД-волюм) + devcontainer chown.
- [ ] go-ci зелёный (build/vet/test-race/golangci/sqlc-drift); тесты store+api.
- [ ] PR (require-pr) → CI зелёный → ревью (workspace-архитектор+user) → мерж. Гейты применяем, когда бэк собирается.

## Handoff'ы (не зона tasker)
- **→ devopser:** `registry/ports.md` — добавить `tasker:8030` (контракт портов; PR-гейт). Координирует workspace-архитектор.
- **→ позже:** universal bridge (foundation-меха) + chater — дизайн ляжет **нодами в tasker** (догфуд), tasker сам себя первым описывает.

## North-star проверка
Модель не рекурсивная (плоский список), key нестабильный (рефы битые), роллап в raw-колонке (не derived),
GraphQL вместо REST-на-дверь, или инфра-костыль вместо фундамента — дефект. Канон рынка + наше дерево, на готовой базе.
