package service

import (
	"context"
	"database/sql"

	"github.com/omnifield/tasker/internal/store"
)

// GetSubtree возвращает узел и всё его поддерево (рекурсивно, обогащённое) — один запрос вместо
// N обходов children. maxDepth < 0 — без ограничения; maxDepth = 0 — только сам узел (без
// раскрытия детей). Кросс-деп рёбра сюда не входят (дерево — parent/child; кросс — relations).
func (s *Service) GetSubtree(ctx context.Context, idOrKey string, maxDepth int) (TreeNode, error) {
	var out TreeNode
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		out, err = s.buildSubtree(ctx, q, n, maxDepth)
		return err
	})
	return out, err
}

// GetWorkspaceTree возвращает корни workspace + их поддеревья (обогащённые). Стартовый экран
// фронта: список workspace → дерево. maxDepth — как в GetSubtree.
func (s *Service) GetWorkspaceTree(ctx context.Context, wsIDOrKey string, maxDepth int) ([]TreeNode, error) {
	out := []TreeNode{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		roots, err := q.ListRootNodes(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, r := range roots {
			t, err := s.buildSubtree(ctx, q, r, maxDepth)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

// buildSubtree рекурсивно обогащает узел и его детей. depth — оставшаяся глубина спуска
// (< 0 — без лимита: depth-1 остаётся отрицательным; = 0 — стоп, дети не раскрываются).
func (s *Service) buildSubtree(ctx context.Context, q *store.Queries, n store.Node, depth int) (TreeNode, error) {
	dto, err := s.enrichNode(ctx, q, n)
	if err != nil {
		return TreeNode{}, err
	}
	node := TreeNode{Node: dto, Children: []TreeNode{}}
	if depth == 0 {
		return node, nil
	}
	kids, err := q.ListChildren(ctx, sql.NullString{String: n.ID, Valid: true})
	if err != nil {
		return TreeNode{}, err
	}
	for _, k := range kids {
		child, err := s.buildSubtree(ctx, q, k, depth-1)
		if err != nil {
			return TreeNode{}, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}
