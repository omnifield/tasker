package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// CreateNodeInput — вход создания узла. ParentID/StatusID принимают UUID ИЛИ key (резолвятся).
type CreateNodeInput struct {
	Title       string
	Description string
	ParentID    *string
	StatusID    *string
	Priority    int64
	Actor       string
}

// UpdateNodeInput — частичный PATCH. Пары Set*/значение различают «поле не прислано» от
// «поле выставлено в null» (очистка parent/status).
type UpdateNodeInput struct {
	Title       *string
	Description *string
	Priority    *int64
	SetStatus   bool
	StatusID    *string // при SetStatus: nil => очистить статус
	SetParent   bool
	ParentID    *string // при SetParent: nil => сделать корнем
	Actor       string
}

// NodeFilter — фильтры списка (накладываются in-memory). *Set=true включает соответствующий фильтр.
type NodeFilter struct {
	ParentSet bool
	ParentID  *string // resolved UUID; nil => только корни
	StatusSet bool
	StatusID  *string // resolved UUID; nil => только без статуса
}

// CreateNode создаёт узел: аллокация стабильного key через транзакционный per-workspace счётчик,
// enforce FK (workspace/parent/status существуют и согласованы), activity=created, webhook create.
func (s *Service) CreateNode(ctx context.Context, wsIDOrKey string, in CreateNodeInput) (Node, error) {
	if in.Title == "" {
		return Node{}, fmt.Errorf("%w: title required", ErrValidation)
	}
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}

		parent, err := s.resolveParentInWorkspace(ctx, q, ws.ID, in.ParentID)
		if err != nil {
			return err
		}
		status, err := s.resolveStatusInWorkspace(ctx, q, ws.ID, in.StatusID)
		if err != nil {
			return err
		}

		seq, err := q.BumpWorkspaceNodeSeq(ctx, store.BumpWorkspaceNodeSeqParams{
			UpdatedAt: s.nowStr(), ID: ws.ID,
		})
		if err != nil {
			return err
		}
		key := ws.Key + "-" + strconv.FormatInt(seq, 10)
		now := s.nowStr()

		node, err := q.CreateNode(ctx, store.CreateNodeParams{
			ID: s.newID(), WorkspaceID: ws.ID, Seq: seq, Key: key,
			ParentID: parent, Title: in.Title, Description: in.Description,
			StatusID: status, Priority: in.Priority,
			Origin: originNative, ProposedBy: "", SourceWs: "",
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, node.ID, in.Actor, "created", map[string]any{"key": key}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, node)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "node", out.Key, out)
	return out, nil
}

// GetNode резолвит узел по UUID или key (dual-id) и обогащает производными полями.
func (s *Service) GetNode(ctx context.Context, idOrKey string) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, n)
		return err
	})
	return out, err
}

// ListNodes возвращает узлы workspace с опц-фильтрами parent/status (in-memory).
func (s *Service) ListNodes(ctx context.Context, wsIDOrKey string, f NodeFilter) ([]Node, error) {
	out := []Node{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		nodes, err := q.ListNodesByWorkspace(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if f.ParentSet && !matchNull(n.ParentID, f.ParentID) {
				continue
			}
			if f.StatusSet && !matchNull(n.StatusID, f.StatusID) {
				continue
			}
			dto, err := s.enrichNode(ctx, q, n)
			if err != nil {
				return err
			}
			out = append(out, dto)
		}
		return nil
	})
	return out, err
}

// ListChildren — прямые дети узла (dual-id родитель), обогащённые.
func (s *Service) ListChildren(ctx context.Context, idOrKey string) ([]Node, error) {
	out := []Node{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		parent, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		kids, err := q.ListChildren(ctx, sql.NullString{String: parent.ID, Valid: true})
		if err != nil {
			return err
		}
		for _, n := range kids {
			dto, err := s.enrichNode(ctx, q, n)
			if err != nil {
				return err
			}
			out = append(out, dto)
		}
		return nil
	})
	return out, err
}

// UpdateNode применяет частичный PATCH, enforce'я FK/цикл и логируя status_changed.
func (s *Service) UpdateNode(ctx context.Context, idOrKey string, in UpdateNodeInput) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		p := store.UpdateNodeParams{
			Title: n.Title, Description: n.Description, StatusID: n.StatusID,
			Priority: n.Priority, ParentID: n.ParentID, UpdatedAt: s.nowStr(), ID: n.ID,
		}
		if in.Title != nil {
			if *in.Title == "" {
				return fmt.Errorf("%w: title cannot be empty", ErrValidation)
			}
			p.Title = *in.Title
		}
		if in.Description != nil {
			p.Description = *in.Description
		}
		if in.Priority != nil {
			p.Priority = *in.Priority
		}
		statusChanged := false
		if in.SetStatus {
			status, err := s.resolveStatusInWorkspace(ctx, q, n.WorkspaceID, in.StatusID)
			if err != nil {
				return err
			}
			statusChanged = p.StatusID.String != status.String || p.StatusID.Valid != status.Valid
			p.StatusID = status
		}
		if in.SetParent {
			parent, err := s.resolveParentForUpdate(ctx, q, n, in.ParentID)
			if err != nil {
				return err
			}
			p.ParentID = parent
		}

		updated, err := q.UpdateNode(ctx, p)
		if err != nil {
			return err
		}
		if statusChanged {
			if err := s.addActivity(ctx, q, updated.ID, in.Actor, "status_changed",
				map[string]any{"status_id": nullToPtr(p.StatusID)}); err != nil {
				return err
			}
		}
		out, err = s.enrichNode(ctx, q, updated)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// DeleteNode удаляет узел. Запрещаем удаление узла с детьми (иначе осиротим поддерево —
// app-level FK enforce). Зависимые строки (relations обеих сторон, метки, ассайни, activity)
// чистим в той же транзакции: БД-FK — RESTRICT, а у узла всегда есть хотя бы activity=created,
// поэтому без чистки удаление падает на FOREIGN KEY constraint.
func (s *Service) DeleteNode(ctx context.Context, idOrKey string) error {
	var deletedKey string
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		cnt, err := q.CountChildren(ctx, sql.NullString{String: n.ID, Valid: true})
		if err != nil {
			return err
		}
		if cnt > 0 {
			return fmt.Errorf("%w: node %s has %d children; reparent or delete them first", ErrConflict, n.Key, cnt)
		}
		// Рёбра-зависимости узла (в обе стороны, в т.ч. cross-workspace) исчезают вместе с ним.
		if err := q.DeleteRelationsForNode(ctx, store.DeleteRelationsForNodeParams{FromNode: n.ID, ToNode: n.ID}); err != nil {
			return err
		}
		if err := q.DeleteNodeLabelsForNode(ctx, n.ID); err != nil {
			return err
		}
		if err := q.DeleteNodeAssigneesForNode(ctx, n.ID); err != nil {
			return err
		}
		if err := q.DeleteActivityForNode(ctx, n.ID); err != nil {
			return err
		}
		deletedKey = n.Key
		return q.DeleteNode(ctx, n.ID)
	})
	if err != nil {
		return err
	}
	s.emit(ctx, webhook.ActionRemove, "node", deletedKey, map[string]string{"key": deletedKey})
	return nil
}

// --- FK/цикл enforce (app-level) -------------------------------------------

// resolveParentInWorkspace: nil-вход -> корень; иначе parent обязан существовать и лежать в том же
// workspace (дерево не пересекает границы workspace; кросс-ws — это relation, не parent_id).
func (s *Service) resolveParentInWorkspace(ctx context.Context, q *store.Queries, wsID string, idOrKey *string) (sql.NullString, error) {
	if idOrKey == nil {
		return sql.NullString{}, nil
	}
	p, err := s.resolveNode(ctx, q, *idOrKey)
	if err != nil {
		return sql.NullString{}, err
	}
	if p.WorkspaceID != wsID {
		return sql.NullString{}, fmt.Errorf("%w: parent %s is in another workspace", ErrValidation, p.Key)
	}
	return sql.NullString{String: p.ID, Valid: true}, nil
}

// resolveParentForUpdate — как выше, плюс запрет self-parent и циклов (parent не может быть
// самим узлом или его потомком).
func (s *Service) resolveParentForUpdate(ctx context.Context, q *store.Queries, node store.Node, idOrKey *string) (sql.NullString, error) {
	if idOrKey == nil {
		return sql.NullString{}, nil
	}
	p, err := s.resolveNode(ctx, q, *idOrKey)
	if err != nil {
		return sql.NullString{}, err
	}
	if p.WorkspaceID != node.WorkspaceID {
		return sql.NullString{}, fmt.Errorf("%w: parent %s is in another workspace", ErrValidation, p.Key)
	}
	if p.ID == node.ID {
		return sql.NullString{}, fmt.Errorf("%w: node cannot be its own parent", ErrValidation)
	}
	// Идём вверх от предполагаемого родителя: встретили node -> цикл.
	cur := p
	for cur.ParentID.Valid {
		if cur.ParentID.String == node.ID {
			return sql.NullString{}, fmt.Errorf("%w: reparent would create a cycle", ErrValidation)
		}
		cur, err = q.GetNodeByID(ctx, cur.ParentID.String)
		if err != nil {
			return sql.NullString{}, err
		}
	}
	return sql.NullString{String: p.ID, Valid: true}, nil
}

// resolveStatusInWorkspace: nil -> без статуса; иначе статус обязан существовать и принадлежать ws.
func (s *Service) resolveStatusInWorkspace(ctx context.Context, q *store.Queries, wsID string, statusID *string) (sql.NullString, error) {
	if statusID == nil {
		return sql.NullString{}, nil
	}
	st, err := q.GetStatus(ctx, *statusID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, fmt.Errorf("%w: status %q not found", ErrValidation, *statusID)
	}
	if err != nil {
		return sql.NullString{}, err
	}
	if st.WorkspaceID != wsID {
		return sql.NullString{}, fmt.Errorf("%w: status %s is in another workspace", ErrValidation, st.ID)
	}
	return sql.NullString{String: st.ID, Valid: true}, nil
}

// --- обогащение + activity + emit ------------------------------------------

// enrichNode добавляет производный Rollup, метки и ассайни к базовому узлу.
func (s *Service) enrichNode(ctx context.Context, q *store.Queries, n store.Node) (Node, error) {
	dto := baseNode(n)
	rollup, err := s.computeRollup(ctx, q, n)
	if err != nil {
		return Node{}, err
	}
	dto.Rollup = rollup

	labels, err := q.ListNodeLabels(ctx, n.ID)
	if err != nil {
		return Node{}, err
	}
	for _, l := range labels {
		dto.Labels = append(dto.Labels, mapLabel(l))
	}
	assignees, err := q.ListNodeAssignees(ctx, n.ID)
	if err != nil {
		return Node{}, err
	}
	dto.Assignees = append(dto.Assignees, assignees...)
	return dto, nil
}

func (s *Service) addActivity(ctx context.Context, q *store.Queries, nodeID, actor, kind string, data any) error {
	raw := ""
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	if actor == "" {
		actor = "system"
	}
	_, err := q.CreateActivity(ctx, store.CreateActivityParams{
		ID: s.newID(), NodeID: nodeID, Actor: actor, Kind: kind, Data: raw, CreatedAt: s.nowStr(),
	})
	return err
}

func (s *Service) emit(ctx context.Context, action webhook.Action, resource, id string, payload any) {
	s.hook.Emit(ctx, webhook.Event{Action: action, Resource: resource, ID: id, Payload: payload})
}

// matchNull: узловое nullable-поле == искомому (nil == «пусто»).
func matchNull(field sql.NullString, want *string) bool {
	if want == nil {
		return !field.Valid
	}
	return field.Valid && field.String == *want
}
