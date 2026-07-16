package service

// Доменные представления (JSON-форма API). Одна канон-схема питает и REST-ответ, и
// webhook-payload: эти структуры и есть зеркало сущности в событии.

import (
	"encoding/json"

	"github.com/omnifield/tasker/internal/store"
)

// Workspace — группа-тир (продукт ИЛИ концерн).
type Workspace struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Status — per-workspace статус: фикс-категория + кастомные name/color.
type Status struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Position    int64  `json:"position"`
	CreatedAt   string `json:"created_at"`
}

// Label — per-workspace метка.
type Label struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	CreatedAt   string `json:"created_at"`
}

// Relation — typed first-class связь (cross-workspace допустима).
type Relation struct {
	ID        string `json:"id"`
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// Activity — запись typed-timeline (kind=commented и т.п.). Data — произвольный JSON-payload.
type Activity struct {
	ID        string          `json:"id"`
	NodeID    string          `json:"node_id"`
	Actor     string          `json:"actor"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// Node — рекурсивный узел + производный Rollup + метки/ассайни. key стабилен/immutable.
type Node struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspace_id"`
	Key         string   `json:"key"`
	Seq         int64    `json:"seq"`
	ParentID    *string  `json:"parent_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	StatusID    *string  `json:"status_id"`
	Priority    int64    `json:"priority"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Rollup      Rollup   `json:"rollup"`
	Labels      []Label  `json:"labels"`
	Assignees   []string `json:"assignees"`
}

// --- мапперы store -> домен -------------------------------------------------

func mapWorkspace(w store.Workspace) Workspace {
	return Workspace{
		ID: w.ID, Key: w.Key, Name: w.Name, Description: w.Description,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

func mapStatus(s store.Status) Status {
	return Status{
		ID: s.ID, WorkspaceID: s.WorkspaceID, Category: s.Category,
		Name: s.Name, Color: s.Color, Position: s.Position, CreatedAt: s.CreatedAt,
	}
}

func mapLabel(l store.Label) Label {
	return Label{
		ID: l.ID, WorkspaceID: l.WorkspaceID, Name: l.Name, Color: l.Color, CreatedAt: l.CreatedAt,
	}
}

func mapRelation(r store.Relation) Relation {
	return Relation{ID: r.ID, FromNode: r.FromNode, ToNode: r.ToNode, Kind: r.Kind, CreatedAt: r.CreatedAt}
}

func mapActivity(a store.Activity) Activity {
	var data json.RawMessage
	if a.Data != "" {
		data = json.RawMessage(a.Data)
	}
	return Activity{
		ID: a.ID, NodeID: a.NodeID, Actor: a.Actor, Kind: a.Kind, Data: data, CreatedAt: a.CreatedAt,
	}
}

// baseNode мапит store.Node в домен БЕЗ производных полей (rollup/labels/assignees заполняет
// enrichNode). Labels/Assignees инициализируем пустыми слайсами -> JSON [] вместо null.
func baseNode(n store.Node) Node {
	return Node{
		ID: n.ID, WorkspaceID: n.WorkspaceID, Key: n.Key, Seq: n.Seq,
		ParentID: nullToPtr(n.ParentID), Title: n.Title, Description: n.Description,
		StatusID: nullToPtr(n.StatusID), Priority: n.Priority,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		Labels: []Label{}, Assignees: []string{},
	}
}
