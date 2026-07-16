package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// key workspace — короткий префикс node.key ("TASK" -> "TASK-1"). Без '-' (иначе ломает разбор
// node.key) и без пробелов; нормализуем в UPPER.
var wsKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// defaultStatuses — сид per-workspace: по одному статусу на каждую фикс-категорию, чтобы узлы
// сразу могли иметь статус, а роллап — читать категории. Owner может добавить кастомные.
var defaultStatuses = []struct {
	Category, Name, Color string
}{
	{"backlog", "Backlog", "#95a5a6"},
	{"todo", "Todo", "#3498db"},
	{"in_progress", "In Progress", "#f1c40f"},
	{"done", "Done", "#2ecc71"},
	{"canceled", "Canceled", "#e74c3c"},
}

// CreateWorkspaceInput — вход создания workspace.
type CreateWorkspaceInput struct {
	Key         string
	Name        string
	Description string
	Actor       string
}

// CreateWorkspace создаёт workspace (+ сид статус-категорий). key уникален, нормализован в UPPER.
func (s *Service) CreateWorkspace(ctx context.Context, in CreateWorkspaceInput) (Workspace, error) {
	key := strings.ToUpper(strings.TrimSpace(in.Key))
	if !wsKeyRe.MatchString(key) {
		return Workspace{}, fmt.Errorf("%w: key must match [A-Z][A-Z0-9]* (no '-', no spaces)", ErrValidation)
	}
	if strings.TrimSpace(in.Name) == "" {
		return Workspace{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	var out Workspace
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetWorkspaceByKey(ctx, key); err == nil {
			return fmt.Errorf("%w: workspace key %q already exists", ErrConflict, key)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := s.nowStr()
		ws, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{
			ID: s.newID(), Key: key, Name: in.Name, Description: in.Description,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		for i, ds := range defaultStatuses {
			if _, err := q.CreateStatus(ctx, store.CreateStatusParams{
				ID: s.newID(), WorkspaceID: ws.ID, Category: ds.Category,
				Name: ds.Name, Color: ds.Color, Position: int64(i), CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		out = mapWorkspace(ws)
		return nil
	})
	if err != nil {
		return Workspace{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "workspace", out.Key, out)
	return out, nil
}

// GetWorkspace — dual-id (UUID или key).
func (s *Service) GetWorkspace(ctx context.Context, idOrKey string) (Workspace, error) {
	var out Workspace
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		w, err := s.resolveWorkspace(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		out = mapWorkspace(w)
		return nil
	})
	return out, err
}

// ListWorkspaces — все workspace.
func (s *Service) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	out := []Workspace{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := q.ListWorkspaces(ctx)
		if err != nil {
			return err
		}
		for _, w := range ws {
			out = append(out, mapWorkspace(w))
		}
		return nil
	})
	return out, err
}

// ListStatuses — статусы workspace (для выбора при set-status узла).
func (s *Service) ListStatuses(ctx context.Context, wsIDOrKey string) ([]Status, error) {
	out := []Status{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		sts, err := q.ListStatusesByWorkspace(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, st := range sts {
			out = append(out, mapStatus(st))
		}
		return nil
	})
	return out, err
}

// CreateStatusInput — кастомный статус поверх сид-набора.
type CreateStatusInput struct {
	Category string
	Name     string
	Color    string
	Position int64
}

// CreateStatus добавляет per-workspace статус (валидируя категорию).
func (s *Service) CreateStatus(ctx context.Context, wsIDOrKey string, in CreateStatusInput) (Status, error) {
	if !statusCategories[in.Category] {
		return Status{}, fmt.Errorf("%w: category must be one of backlog|todo|in_progress|done|canceled", ErrValidation)
	}
	if strings.TrimSpace(in.Name) == "" {
		return Status{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	var out Status
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		st, err := q.CreateStatus(ctx, store.CreateStatusParams{
			ID: s.newID(), WorkspaceID: ws.ID, Category: in.Category,
			Name: in.Name, Color: in.Color, Position: in.Position, CreatedAt: s.nowStr(),
		})
		if err != nil {
			return err
		}
		out = mapStatus(st)
		return nil
	})
	return out, err
}
