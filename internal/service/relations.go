package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// CreateRelationInput — typed-связь между узлами. from/to принимают UUID или key. cross-workspace
// ДОПУСТИМ (в отличие от parent_id). kind ∈ blocks|relates|duplicate.
type CreateRelationInput struct {
	ToNode string // целевой узел (id или key)
	Kind   string
	Actor  string
}

// AddRelation создаёт связь fromNode -> in.ToNode. Enforce: оба узла существуют, kind валиден,
// не self-link. Логирует relation_added у источника, emit create.
func (s *Service) AddRelation(ctx context.Context, fromIDOrKey string, in CreateRelationInput) (Relation, error) {
	if !relationKinds[in.Kind] {
		return Relation{}, fmt.Errorf("%w: kind must be one of blocks|relates|duplicate", ErrValidation)
	}
	var out Relation
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		from, err := s.resolveNode(ctx, q, fromIDOrKey)
		if err != nil {
			return err
		}
		to, err := s.resolveNode(ctx, q, in.ToNode)
		if err != nil {
			return err
		}
		if from.ID == to.ID {
			return fmt.Errorf("%w: cannot relate a node to itself", ErrValidation)
		}
		rel, err := q.CreateRelation(ctx, store.CreateRelationParams{
			ID: s.newID(), FromNode: from.ID, ToNode: to.ID, Kind: in.Kind, CreatedAt: s.nowStr(),
		})
		if err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, from.ID, in.Actor, "relation_added",
			map[string]any{"kind": in.Kind, "to": to.Key}); err != nil {
			return err
		}
		out = mapRelation(rel)
		return nil
	})
	if err != nil {
		return Relation{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "relation", out.ID, out)
	return out, nil
}

// ListRelations — все связи узла (как источник И как цель), ОБОГАЩЁННЫЕ: направление
// относительно узла + чужой конец (ws/key/title) + cross-ws флаг. Рёбра-зависимости НЕ
// роллапятся (роллап — внутри дерева parent/child; кросс — только здесь).
func (s *Service) ListRelations(ctx context.Context, idOrKey string) ([]RelationView, error) {
	out := []RelationView{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		rels, err := q.ListRelationsForNode(ctx, store.ListRelationsForNodeParams{FromNode: n.ID, ToNode: n.ID})
		if err != nil {
			return err
		}
		for _, r := range rels {
			view, err := s.enrichRelation(ctx, q, n, r)
			if err != nil {
				return err
			}
			out = append(out, view)
		}
		return nil
	})
	return out, err
}

// enrichRelation обогащает связь относительно узла self: направление (self=from -> outgoing,
// self=to -> incoming), «другой конец» с его workspace/key/title и cross-workspace флаг.
func (s *Service) enrichRelation(ctx context.Context, q *store.Queries, self store.Node, r store.Relation) (RelationView, error) {
	otherID, direction := r.ToNode, "outgoing"
	if r.ToNode == self.ID {
		otherID, direction = r.FromNode, "incoming"
	}
	other, err := q.GetNodeByID(ctx, otherID)
	if err != nil {
		return RelationView{}, err
	}
	ws, err := q.GetWorkspace(ctx, other.WorkspaceID)
	if err != nil {
		return RelationView{}, err
	}
	return RelationView{
		ID: r.ID, Kind: r.Kind, Direction: direction,
		FromNode: r.FromNode, ToNode: r.ToNode,
		Other: RelatedNode{
			ID: other.ID, Key: other.Key, Title: other.Title,
			WorkspaceID: other.WorkspaceID, WorkspaceKey: ws.Key,
		},
		CrossWorkspace: other.WorkspaceID != self.WorkspaceID,
		CreatedAt:      r.CreatedAt,
	}, nil
}

// DeleteRelation удаляет связь по id.
func (s *Service) DeleteRelation(ctx context.Context, relID string) error {
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetRelation(ctx, relID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: relation %q", ErrNotFound, relID)
		} else if err != nil {
			return err
		}
		return q.DeleteRelation(ctx, relID)
	})
	if err != nil {
		return err
	}
	s.emit(ctx, webhook.ActionRemove, "relation", relID, map[string]string{"id": relID})
	return nil
}
