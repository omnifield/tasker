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

// Relation — typed first-class связь (cross-workspace допустима). Сырая форма (ответ create).
type Relation struct {
	ID        string `json:"id"`
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// RelatedNode — снимок «другого конца» связи: чужой узел с workspace/key/title, чтобы срез
// показал «заблокирован узлом INFRA-42 (чужой)» без второго запроса.
type RelatedNode struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Title        string `json:"title"`
	WorkspaceID  string `json:"workspace_id"`
	WorkspaceKey string `json:"workspace_key"`
}

// RelationView — обогащённая связь ОТНОСИТЕЛЬНО запрошенного узла: направление (outgoing =
// этот узел → other; incoming = other → этот узел — для направленных blocks/depends_on даёт
// однозначное «A зависит от B»), обогащённый «другой конец» и явный cross-workspace флаг.
type RelationView struct {
	ID             string      `json:"id"`
	Kind           string      `json:"kind"`
	Direction      string      `json:"direction"`
	FromNode       string      `json:"from_node"`
	ToNode         string      `json:"to_node"`
	Other          RelatedNode `json:"other"`
	CrossWorkspace bool        `json:"cross_workspace"`
	CreatedAt      string      `json:"created_at"`
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
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Key         string  `json:"key"`
	Seq         int64   `json:"seq"`
	ParentID    *string `json:"parent_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	StatusID    *string `json:"status_id"`
	Priority    int64   `json:"priority"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	// Origin: "native" (roadmap node) or "proposal" (cross-product suggestion in the inbox,
	// not yet accepted into the roadmap). ProposedBy/SourceWs carry the proposal's provenance.
	Origin     string   `json:"origin"`
	ProposedBy string   `json:"proposed_by,omitempty"`
	SourceWs   string   `json:"source_ws,omitempty"`
	Rollup     Rollup   `json:"rollup"`
	Labels     []Label  `json:"labels"`
	Assignees  []string `json:"assignees"`
}

// TreeNode — узел + рекурсивно его поддерево (subtree-fetch: всё дерево одним запросом для
// рисовки/догфуд-навигации). Каждый узел обогащён (rollup/labels/assignees); rollup.total —
// число прямых детей. Кросс-деп рёбра сюда НЕ входят (только parent/child; кросс — relations).
type TreeNode struct {
	Node
	Children []TreeNode `json:"children"`
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
		Origin: n.Origin, ProposedBy: n.ProposedBy, SourceWs: n.SourceWs,
		Labels: []Label{}, Assignees: []string{},
	}
}
