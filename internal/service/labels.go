package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// CreateLabelInput — per-workspace метка.
type CreateLabelInput struct {
	Name  string
	Color string
}

// CreateLabel создаёт метку в workspace (name уникален в пределах ws — enforce'ит БД-констрейнт).
func (s *Service) CreateLabel(ctx context.Context, wsIDOrKey string, in CreateLabelInput) (Label, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Label{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	var out Label
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		l, err := q.CreateLabel(ctx, store.CreateLabelParams{
			ID: s.newID(), WorkspaceID: ws.ID, Name: in.Name, Color: in.Color, CreatedAt: s.nowStr(),
		})
		if err != nil {
			return err
		}
		out = mapLabel(l)
		return nil
	})
	if err != nil {
		return Label{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "label", out.ID, out)
	return out, nil
}

// ListLabels — метки workspace.
func (s *Service) ListLabels(ctx context.Context, wsIDOrKey string) ([]Label, error) {
	out := []Label{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		ls, err := q.ListLabelsByWorkspace(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, l := range ls {
			out = append(out, mapLabel(l))
		}
		return nil
	})
	return out, err
}

// AddNodeLabel навешивает метку на узел (enforce: узел и метка существуют и в одном workspace).
func (s *Service) AddNodeLabel(ctx context.Context, nodeIDOrKey, labelID string) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, nodeIDOrKey)
		if err != nil {
			return err
		}
		l, err := q.GetLabel(ctx, labelID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: label %q", ErrNotFound, labelID)
		} else if err != nil {
			return err
		}
		if l.WorkspaceID != n.WorkspaceID {
			return fmt.Errorf("%w: label %s is in another workspace", ErrValidation, l.ID)
		}
		if err := q.AddNodeLabel(ctx, store.AddNodeLabelParams{NodeID: n.ID, LabelID: l.ID}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, n)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// RemoveNodeLabel снимает метку с узла.
func (s *Service) RemoveNodeLabel(ctx context.Context, nodeIDOrKey, labelID string) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, nodeIDOrKey)
		if err != nil {
			return err
		}
		if err := q.RemoveNodeLabel(ctx, store.RemoveNodeLabelParams{NodeID: n.ID, LabelID: labelID}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, n)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// AddAssignee назначает актора (агент/человек) на узел; логирует assigned.
func (s *Service) AddAssignee(ctx context.Context, nodeIDOrKey, assignee, actor string) (Node, error) {
	if strings.TrimSpace(assignee) == "" {
		return Node{}, fmt.Errorf("%w: assignee required", ErrValidation)
	}
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, nodeIDOrKey)
		if err != nil {
			return err
		}
		if err := q.AddNodeAssignee(ctx, store.AddNodeAssigneeParams{
			NodeID: n.ID, Actor: assignee, CreatedAt: s.nowStr(),
		}); err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, n.ID, actor, "assigned", map[string]any{"assignee": assignee}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, n)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// RemoveAssignee снимает актора с узла.
func (s *Service) RemoveAssignee(ctx context.Context, nodeIDOrKey, assignee string) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, nodeIDOrKey)
		if err != nil {
			return err
		}
		if err := q.RemoveNodeAssignee(ctx, store.RemoveNodeAssigneeParams{NodeID: n.ID, Actor: assignee}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, n)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}
