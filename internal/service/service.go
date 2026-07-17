// Package service — доменное ядро tasker: рекурсивное дерево узлов, dual-id резолв,
// стабильный per-workspace key, typed relations, activity-timeline и РОЛЛАП-DERIVED
// (прогресс/статус родителя вычисляется из детей на чтении — сервис владеет им, не raw-колонка).
// FK enforce'им ЗДЕСЬ (Jira-урок: не полагаться на логические FK БД). Мутации зеркалятся
// в webhook-события канон-формы.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// Класс-ошибки: httpapi маппит их в статус-коды (404/400/409). Детали — через %w-обёртку.
var (
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation")
	ErrConflict   = errors.New("conflict")
)

// Валидные фикс-категории статуса (пол статуса может быть бинарным, но категория — канон).
var statusCategories = map[string]bool{
	"backlog": true, "todo": true, "in_progress": true, "done": true, "canceled": true,
}

// Валидные типы typed-relation (parent/child идёт через parent_id, НЕ сюда). Направленные
// (from -> to): `blocks` (from блокирует to) и `depends_on` (from зависит от to — роадмап-ребро
// на чужой/инфра-узел). Ненаправленные: `relates`, `duplicate`. Кросс-workspace допустим для всех.
var relationKinds = map[string]bool{
	"blocks": true, "depends_on": true, "relates": true, "duplicate": true,
}

// Service — фасад над store + webhook. now/newID инъектируемы для детерминизма в тестах.
type Service struct {
	store *store.Store
	hook  webhook.Emitter
	now   func() time.Time
	newID func() string
}

// New собирает сервис с дефолтным временем (UTC) и UUID-генератором.
func New(st *store.Store, hook webhook.Emitter) *Service {
	if hook == nil {
		hook = webhook.Nop{}
	}
	return &Service{
		store: st,
		hook:  hook,
		now:   func() time.Time { return time.Now().UTC() },
		newID: uuid.NewString,
	}
}

// nowStr — таймстемп в RFC3339Nano UTC (лексикографический порядок == хронологический).
func (s *Service) nowStr() string { return s.now().Format(time.RFC3339Nano) }

// --- helpers null<->ptr ----------------------------------------------------

func nullToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// --- dual-id резолв узла ---------------------------------------------------

// resolveNode находит узел по UUID ИЛИ по стабильному key ("WS-123") — оба резолвят один узел.
// UUID и key не пересекаются по формату, поэтому порядок проб безопасен.
func (s *Service) resolveNode(ctx context.Context, q *store.Queries, idOrKey string) (store.Node, error) {
	n, err := q.GetNodeByID(ctx, idOrKey)
	if err == nil {
		return n, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.Node{}, err
	}
	n, err = q.GetNodeByKey(ctx, idOrKey)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Node{}, fmt.Errorf("%w: node %q", ErrNotFound, idOrKey)
	}
	if err != nil {
		return store.Node{}, err
	}
	return n, nil
}

// resolveWorkspace — по UUID или короткому key.
func (s *Service) resolveWorkspace(ctx context.Context, q *store.Queries, idOrKey string) (store.Workspace, error) {
	w, err := q.GetWorkspace(ctx, idOrKey)
	if err == nil {
		return w, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.Workspace{}, err
	}
	w, err = q.GetWorkspaceByKey(ctx, idOrKey)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Workspace{}, fmt.Errorf("%w: workspace %q", ErrNotFound, idOrKey)
	}
	if err != nil {
		return store.Workspace{}, err
	}
	return w, nil
}

// --- роллап (DERIVED, read-time) -------------------------------------------

// Rollup — производный агрегат по ПРЯМЫМ детям узла. Никогда не хранится: вычисляется здесь
// при чтении (north-star: не raw-колонка). Форма зафиксирована минимально, но явно.
type Rollup struct {
	HasChildren bool    `json:"has_children"`
	Total       int     `json:"total"`     // число прямых детей
	Completed   int     `json:"completed"` // дети в категории done|canceled
	Fraction    float64 `json:"fraction"`  // completed/total (0 при отсутствии детей)
	Category    string  `json:"category"`  // свёрнутая категория (см. правило ниже)
}

// computeRollup читает категории прямых детей и сворачивает их по явному правилу:
//   - нет детей         -> отражает СОБСТВЕННУЮ категорию узла (done => fraction 1.0)
//   - все canceled      -> canceled
//   - completed == total-> done
//   - есть in_progress
//     ИЛИ 0<completed   -> in_progress
//   - все backlog       -> backlog
//   - иначе             -> todo
//
// Дети без статуса трактуются как backlog (незапущено). Роллап — прямой (не рекурсивный):
// минимально и явно; глубокая свёртка — осознанная будущая итерация.
func (s *Service) computeRollup(ctx context.Context, q *store.Queries, n store.Node) (Rollup, error) {
	rows, err := q.ListChildStatusCategories(ctx, sql.NullString{String: n.ID, Valid: true})
	if err != nil {
		return Rollup{}, err
	}
	counts := map[string]int{}
	total := 0
	for _, r := range rows {
		cat := "backlog"
		if r.Category.Valid && r.Category.String != "" {
			cat = r.Category.String
		}
		counts[cat] += int(r.Cnt)
		total += int(r.Cnt)
	}

	if total == 0 {
		own := s.ownCategory(ctx, q, n)
		frac := 0.0
		if own == "done" || own == "canceled" {
			frac = 1.0
		}
		return Rollup{HasChildren: false, Total: 0, Completed: 0, Fraction: frac, Category: own}, nil
	}

	completed := counts["done"] + counts["canceled"]
	cat := "todo"
	switch {
	case counts["canceled"] == total:
		cat = "canceled"
	case completed == total:
		cat = "done"
	case counts["in_progress"] > 0 || completed > 0:
		cat = "in_progress"
	case counts["backlog"] == total:
		cat = "backlog"
	}
	return Rollup{
		HasChildren: true,
		Total:       total,
		Completed:   completed,
		Fraction:    float64(completed) / float64(total),
		Category:    cat,
	}, nil
}

// ownCategory — категория собственного статуса узла ("backlog", если статус не задан/не найден).
func (s *Service) ownCategory(ctx context.Context, q *store.Queries, n store.Node) string {
	if !n.StatusID.Valid {
		return "backlog"
	}
	st, err := q.GetStatus(ctx, n.StatusID.String)
	if err != nil {
		return "backlog"
	}
	return st.Category
}
