# tasker REST API — контракт для ведения продукта

Практический контракт: как архитектор продукта (**человек или агент — одинаково**) заводит и ведёт
свой роадмап в tasker через API. Одна канон-схема питает и ответы, и webhook-события.

> Спеки `openapi.json` пока нет — этот файл источник правды по контракту. Обзор эндпоинтов также
> в [README](../README.md) (раздел «REST API»).

## База и слой

Все роуты — под нативным префиксом **`/tasker/`** (частая ошибка: дёргать `/nodes` или `/health` →
Go-шный `404 page not found`).

| Слой | База | Когда |
|---|---|---|
| Напрямую в бэк | `http://tasker:8030/tasker/…` | по docker-сети (gateway net); дверь не нужна — **дефолт** |
| Через дверь хаба | `http://gateway/api/tasker/…` | публичный вход; gateway снимает `/api` → тот же `/tasker/…`. Нужна регенерация двери |

Публичный префикс из манифеста — ровно `/api/tasker`. UUID и стабильный `key` взаимозаменяемы везде,
где в пути `{ws}` или `{key}` (**dual-id**).

## Auth

Всё, кроме `GET /tasker/healthz`, требует заголовок:

```
Authorization: Bearer <handle>
```

Токен-стаб: любой непустой `<handle>` проходит (реальная identity — позже, через мост). `handle`
пишется как **actor** в activity — бери осмысленный (`Bearer devopser`), чтобы в истории было видно,
кто вёл. Без токена → `401 {"error":"missing bearer token"}`.

## Модель

`workspace → node` (рекурсия через `parent_id`, любая глубина).

- **Workspace** — продукт ИЛИ концерн. `key` короткий, `^[A-Z][A-Z0-9]*$` (UPPER, без `-`/пробелов).
  При создании авто-сидятся 5 статусов (по одному на категорию).
- **Node** — рекурсивный type-less узел. `key` = `<WS>-<n>` генерится сам, **стабильный/immutable**
  (по нему ссылаются извне; при переносе не переиндексируется).
- **Документация ноды** — сейчас одно текстовое поле `description`. **Отдельной сущности «блок» пока
  нет** — структурные блоки (идея/концепция/рефы-на-каноны/код) в проработке (пресет); до этого
  документация живёт секциями внутри `description`. Мигрируем, когда блочная модель приземлится.

### Правила (enforce в сервис-слое)
- `parent` ноды — только в **своём** workspace (дерево не пересекает границы ws; кросс-ws — это relation).
- Нельзя удалить ноду **с детьми** (сперва reparent/удали детей) → `409`.
- Статус ноды — из статусов её workspace; категории фиксированы:
  `backlog · todo · in_progress · done · canceled`.
- Связи (`relations`) — типизированы, **cross-workspace допустимы**, в дерево-роллап НЕ входят.

## Эндпоинты

### Workspaces
| Метод · путь | Payload / примечание |
|---|---|
| `GET /tasker/workspaces` | список |
| `POST /tasker/workspaces` | `{"key":"DEV","name":"devopser","description":"…"}` → `201` Workspace |
| `GET /tasker/workspaces/{ws}` | один (dual-id) |
| `GET /tasker/workspaces/{ws}/tree?depth=N` | корни ws + поддеревья (обогащённые); `depth` опц. |

### Statuses (взять `status_id` для ноды)
| Метод · путь | Payload |
|---|---|
| `GET /tasker/workspaces/{ws}/statuses` | 5 авто-статусов + кастомные |
| `POST /tasker/workspaces/{ws}/statuses` | `{"category":"in_progress","name":"…","color":"#…","position":0}` |

### Nodes
| Метод · путь | Payload / примечание |
|---|---|
| `GET /tasker/workspaces/{ws}/nodes?parent=<key\|none>&status=<id\|none>` | список с фильтрами |
| `POST /tasker/workspaces/{ws}/nodes` | `{"title":"…"(required),"description":"…","parent_id":"DEV-1","status_id":"<id>","priority":1}` → `201` Node |
| `GET /tasker/nodes/{key}` | нода (dual-id) |
| `PATCH /tasker/nodes/{key}` | partial: любое из `{"title","description","priority","status_id","parent_id"}`; `null` очищает status/parent |
| `DELETE /tasker/nodes/{key}` | `204`; с детьми → `409` |
| `GET /tasker/nodes/{key}/children` | прямые дети |
| `GET /tasker/nodes/{key}/tree?depth=N` | узел + всё поддерево одним запросом |

`parent_id` / `status_id` принимают UUID **или** key.

### Relations (зависимости, в т.ч. кросс-продукт)
| Метод · путь | Payload |
|---|---|
| `GET /tasker/nodes/{key}/relations` | обогащённые: направление + чужой ws/key/title + `cross_workspace` |
| `POST /tasker/nodes/{key}/relations` | `{"to_node":"INFRA-1","kind":"depends_on"}` |
| `DELETE /tasker/relations/{id}` | `204` |

`kind ∈ blocks · depends_on · relates · duplicate`. Направленные (`from → to`): `blocks` (from блокирует
to), `depends_on` (from зависит от to). `to_node` — UUID или key любого workspace.

### Activity / Labels / Assignees
| Метод · путь | Payload |
|---|---|
| `GET /tasker/nodes/{key}/activity` | typed timeline |
| `POST /tasker/nodes/{key}/activity` | `{"kind":"commented","data":{"text":"…"}}` |
| `GET·POST /tasker/workspaces/{ws}/labels` | `POST {"name":"ci","color":"#8e44ad"}` |
| `POST /tasker/nodes/{key}/labels` · `DELETE /…/labels/{label_id}` | `{"label_id":"…"}` навесить · снять |
| `POST /tasker/nodes/{key}/assignees` · `DELETE /…/assignees/{actor}` | `{"assignee":"devopser"}` |

### Health
`GET /tasker/healthz` → `200 {"status":"ok","service":"tasker"}` (без auth).

## Формы ответов (ключевое)

**Node** (и элемент дерева — плюс `children[]`):
```json
{
  "id":"uuid","workspace_id":"uuid","key":"DEV-2","seq":2,
  "parent_id":"uuid|null","title":"…","description":"…",
  "status_id":"uuid|null","priority":1,
  "created_at":"RFC3339","updated_at":"RFC3339",
  "rollup":{"has_children":true,"total":4,"completed":1,"fraction":0.25,"category":"in_progress"},
  "labels":[{"id":"…","name":"ci","color":"#…"}],
  "assignees":["devopser"]
}
```
`rollup` — **derived** (свёртка по прямым детям), не хранится.

**RelationView** (относительно запрошенной ноды):
```json
{
  "id":"uuid","kind":"depends_on","direction":"outgoing|incoming",
  "from_node":"uuid","to_node":"uuid",
  "other":{"id":"uuid","key":"INFRA-1","title":"universal-bridge","workspace_id":"uuid","workspace_key":"INFRA"},
  "cross_workspace":true,"created_at":"RFC3339"
}
```

## Ошибки
| Код | Когда |
|---|---|
| `400` | валидация (пустой title, кривой ws-key, неизвестная категория/kind, parent в чужом ws) |
| `401` | нет/пустой Bearer |
| `404` | нода/workspace не найдены |
| `409` | конфликт (дубль ws-key; удаление ноды с детьми) |

Тело ошибки: `{"error":"…"}`.

## Quickstart

```sh
BASE=http://tasker:8030/tasker
AUTH="Authorization: Bearer devopser"

# 1) workspace (+ авто-статусы)
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" $BASE/workspaces \
  -d '{"key":"DEV","name":"devopser","description":"Роадмап devopser."}'

# 2) status_id по категориям
curl -s -H "$AUTH" $BASE/workspaces/DEV/statuses

# 3) корень → ветка (parent по key)
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" $BASE/workspaces/DEV/nodes \
  -d '{"title":"Роадмап devopser","priority":1}'
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" $BASE/workspaces/DEV/nodes \
  -d '{"title":"Аудит-фиксы","parent_id":"DEV-1","status_id":"<in_progress-id>"}'

# 4) кросс-продукт зависимость + коммент
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" $BASE/nodes/DEV-2/relations \
  -d '{"to_node":"INFRA-1","kind":"depends_on"}'
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" $BASE/nodes/DEV-2/activity \
  -d '{"kind":"commented","data":{"text":"Ждём мост от инфры."}}'

# 5) прочитать дерево
curl -s -H "$AUTH" $BASE/workspaces/DEV/tree
```
