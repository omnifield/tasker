# tasker

**Durable «дерево разработки» — источник правды дизайна продуктов.** Рекурсивный узел
(концепт → механики → каждая нода продумана и задокументирована), роллап-derived прогресс,
стабильные ссылки. Сейчас — бэк-фундамент: модель узла/дерева + REST API на готовой инфре.
Позже: chater даёт разговорный слой (брифы = рефы на ноды tasker), UI — на своём фреймворке.

Опора — `hub/patterns/task-manager-canon.md` (рыночный канон) + `briefs/tasker-founding-backend-v0.md`.

## Модель

- **Node** — рекурсивный type-less core (Issue-канон). `parent_id` (self-FK, nullable) = дерево.
  `key` = `<WS>-<n>` — **стабильный/immutable**, по нему рефают извне (не ре-индексим при переносе).
- **Dual-id** — узел резолвится и по UUID, и по `key` (оба в одном эндпоинте). `key` генерится
  транзакционным per-workspace счётчиком (`unique(workspace_id, seq)`, монотонный).
- **Роллап (derived)** — прогресс/статус родителя вычисляется из **прямых детей** на чтении
  (владеет сервис-слой, не raw-колонка). Форма: `{has_children,total,completed,fraction,category}`.
- **Workspace** — группа-тир (продукт ИЛИ концерн), короткий `key`.
- **Status** — per-workspace, фикс-категория (`backlog|todo|in_progress|done|canceled`) + name/color.
  Сид по одному статусу на категорию при создании workspace.
- **Relation** — typed first-class связь (`blocks|depends_on|relates|duplicate`), **cross-workspace
  допустима** (роадмап-ребро на чужой/инфра-узел: `depends_on` направлен `from → to`). parent/child —
  через `parent_id`, НЕ через relation. Рёбра-зависимости НЕ роллапятся (роллап — внутри дерева).
- **Label** (m2m) + **Assignee** (актор: агент/человек).
- **Activity** — typed timeline с 1-го дня (`created|status_changed|assigned|relation_added|commented`).

FK enforce'им в сервис-слое (Jira-урок: не полагаться на логические FK), плюс объявлены в DDL.

## REST API (`/tasker/*`)

Дверь omnifield-hub: `/api/tasker/…` → rewrite → `tasker:8030/tasker/…`. Auth — token-stub
(`Authorization: Bearer <handle>`); `/tasker/healthz` без auth. Bind `0.0.0.0` (G1).

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/tasker/healthz` | health |
| GET·POST | `/tasker/workspaces` | список · создать |
| GET | `/tasker/workspaces/{ws}` | workspace (dual-id) |
| GET·POST | `/tasker/workspaces/{ws}/nodes` | список (фильтры `?parent=`, `?status=`) · создать |
| GET | `/tasker/workspaces/{ws}/tree` | корни ws + поддеревья (обогащённые; `?depth=N`) |
| GET·POST | `/tasker/workspaces/{ws}/statuses` | статусы · создать |
| GET·POST | `/tasker/workspaces/{ws}/labels` | метки · создать |
| GET·PATCH·DELETE | `/tasker/nodes/{key}` | узел (dual-id: UUID или key) |
| GET | `/tasker/nodes/{key}/children` | прямые дети |
| GET | `/tasker/nodes/{key}/tree` | узел + всё поддерево одним запросом (`?depth=N`) |
| GET·POST | `/tasker/nodes/{key}/relations` | связи (обогащённые: направление + чужой ws/key/title + cross-ws) · добавить |
| GET·POST | `/tasker/nodes/{key}/activity` | timeline · добавить (коммент) |
| POST | `/tasker/nodes/{key}/labels` · DELETE `/…/labels/{id}` | навесить · снять метку |
| POST | `/tasker/nodes/{key}/assignees` · DELETE `/…/assignees/{actor}` | назначить · снять |
| DELETE | `/tasker/relations/{id}` | удалить связь |

Одна канон-схема питает и REST-ответ, и **webhook-payload** (события `create/update/remove`
зеркалят сущность). v0-sink логирует; полноценная доставка — итерация 2.

## Фронт (`web/`, read-first)

Минимальный обзор «видеть дерево»: список workspaces → дерево узлов (subtree-fetch одним
запросом) + роллап-бейджи + кросс-деп рёбра («зависит от `WS-key` (чужой)»). Только просмотр;
создание/правка — следующая итерация. Стек — **vite + solid** на `@omnifield/vite-preset`
(`base "/tasker/"` из манифеста, ноль хардкода). API — через дверь-контракт `/api/tasker/…`,
token-stub `Bearer`.

```sh
pnpm -C web install
pnpm -C web dev       # vite :5173 (base /tasker/); devbox: сервис `frontend`
pnpm -C web test      # vitest (чистые хелперы); lint · typecheck · build — web-ci
```

## Запуск

Конфиг — env-only: `TASKER_PORT` (дефолт `8030`), `TASKER_DB` (путь к SQLite; в devbox —
per-product волюм `/data/tasker/tasker.db`). Миграции (goose, встроены) прогоняются идемпотентно
на старте.

```sh
TASKER_PORT=8030 TASKER_DB=/data/tasker/tasker.db go run ./cmd/tasker
# devbox: node scripts/devbox-services.mjs up   (декларация — devbox.services.json)
```

## Персистентность

Go + **sqlc** (типы из SQL) + **goose** (миграции). SQL — совместимый **SQLite↔Postgres**
(`TEXT`/`INTEGER`, без AUTOINCREMENT/engine-специфики); PG — drop-in-таргет (engine `postgresql`
+ `pgx/v5`). Драйвер сейчас — pure-Go `modernc.org/sqlite` (без cgo).

## Раскладка

```
cmd/tasker/        main (graceful shutdown, slog)
internal/config    env-конфиг
internal/store     sqlc-ядро (queries/ + generated) + Open/Tx + миграции
internal/service   доменное ядро: dual-id, роллап-derived, FK-enforce, activity, webhook-emit
internal/httpapi   REST-слой /tasker/*, auth-stub
internal/webhook   форма событий (канон), log-sink
migrations/        goose *.sql (встроены через embed)
```
