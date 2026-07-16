package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// AddActivityInput — ручная запись в timeline (комментарий = kind "commented" по умолчанию).
type AddActivityInput struct {
	Kind  string
	Data  json.RawMessage
	Actor string
}

// ListActivity — timeline узла в хронологическом порядке.
func (s *Service) ListActivity(ctx context.Context, idOrKey string) ([]Activity, error) {
	out := []Activity{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		acts, err := q.ListActivityForNode(ctx, n.ID)
		if err != nil {
			return err
		}
		for _, a := range acts {
			out = append(out, mapActivity(a))
		}
		return nil
	})
	return out, err
}

// AddActivity добавляет запись в timeline (напр. комментарий). Пустой kind -> "commented".
func (s *Service) AddActivity(ctx context.Context, idOrKey string, in AddActivityInput) (Activity, error) {
	kind := in.Kind
	if kind == "" {
		kind = "commented"
	}
	raw := ""
	if len(in.Data) > 0 {
		if !json.Valid(in.Data) {
			return Activity{}, fmt.Errorf("%w: data must be valid JSON", ErrValidation)
		}
		raw = string(in.Data)
	}
	var out Activity
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		actor := in.Actor
		if actor == "" {
			actor = "system"
		}
		a, err := q.CreateActivity(ctx, store.CreateActivityParams{
			ID: s.newID(), NodeID: n.ID, Actor: actor, Kind: kind, Data: raw, CreatedAt: s.nowStr(),
		})
		if err != nil {
			return err
		}
		out = mapActivity(a)
		return nil
	})
	if err != nil {
		return Activity{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "activity", out.ID, out)
	return out, nil
}
