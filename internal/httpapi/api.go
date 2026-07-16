// Package httpapi — REST-дверь tasker под нативным префиксом /tasker/ (дверь omnifield-hub
// rewrite'ит /api/tasker -> tasker:8030/tasker/). Ложится на webhooks (форма событий — в service).
// Auth — token-stub (Bearer <handle>), как chater v0. Одна канон-схема питает и ответы, и события.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/omnifield/tasker/internal/service"
)

// API — HTTP-слой поверх сервис-ядра.
type API struct {
	svc *service.Service
	log *slog.Logger
}

// New собирает API.
func New(svc *service.Service, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	return &API{svc: svc, log: log}
}

type ctxKey int

const actorKey ctxKey = iota

// Handler возвращает роутер со всеми маршрутами /tasker/* (Go 1.22 method+path patterns).
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	// health — без auth (probe девбокса дергает без токена).
	mux.HandleFunc("GET /tasker/healthz", a.handleHealthz)

	// workspaces
	mux.Handle("GET /tasker/workspaces", a.auth(a.handleListWorkspaces))
	mux.Handle("POST /tasker/workspaces", a.auth(a.handleCreateWorkspace))
	mux.Handle("GET /tasker/workspaces/{ws}", a.auth(a.handleGetWorkspace))
	mux.Handle("GET /tasker/workspaces/{ws}/nodes", a.auth(a.handleListNodes))
	mux.Handle("POST /tasker/workspaces/{ws}/nodes", a.auth(a.handleCreateNode))
	mux.Handle("GET /tasker/workspaces/{ws}/labels", a.auth(a.handleListLabels))
	mux.Handle("POST /tasker/workspaces/{ws}/labels", a.auth(a.handleCreateLabel))
	mux.Handle("GET /tasker/workspaces/{ws}/statuses", a.auth(a.handleListStatuses))
	mux.Handle("POST /tasker/workspaces/{ws}/statuses", a.auth(a.handleCreateStatus))

	// nodes (dual-id: {key} = UUID или стабильный key)
	mux.Handle("GET /tasker/nodes/{key}", a.auth(a.handleGetNode))
	mux.Handle("PATCH /tasker/nodes/{key}", a.auth(a.handleUpdateNode))
	mux.Handle("DELETE /tasker/nodes/{key}", a.auth(a.handleDeleteNode))
	mux.Handle("GET /tasker/nodes/{key}/children", a.auth(a.handleListChildren))
	mux.Handle("GET /tasker/nodes/{key}/relations", a.auth(a.handleListRelations))
	mux.Handle("POST /tasker/nodes/{key}/relations", a.auth(a.handleCreateRelation))
	mux.Handle("GET /tasker/nodes/{key}/activity", a.auth(a.handleListActivity))
	mux.Handle("POST /tasker/nodes/{key}/activity", a.auth(a.handleCreateActivity))
	mux.Handle("POST /tasker/nodes/{key}/labels", a.auth(a.handleAttachLabel))
	mux.Handle("DELETE /tasker/nodes/{key}/labels/{label_id}", a.auth(a.handleDetachLabel))
	mux.Handle("POST /tasker/nodes/{key}/assignees", a.auth(a.handleAddAssignee))
	mux.Handle("DELETE /tasker/nodes/{key}/assignees/{actor}", a.auth(a.handleRemoveAssignee))

	// relations
	mux.Handle("DELETE /tasker/relations/{id}", a.auth(a.handleDeleteRelation))

	return mux
}

func (a *API) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "tasker"})
}

// auth — token-stub middleware: требует `Authorization: Bearer <handle>`, кладёт handle как actor
// в контекст. Пустой/кривой заголовок -> 401. Полноценная identity — позже (мост).
func (a *API) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		const p = "Bearer "
		if !strings.HasPrefix(h, p) {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		handle := strings.TrimSpace(strings.TrimPrefix(h, p))
		if handle == "" {
			writeError(w, http.StatusUnauthorized, "empty bearer handle")
			return
		}
		ctx := context.WithValue(r.Context(), actorKey, handle)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func actorOf(r *http.Request) string {
	if v, ok := r.Context().Value(actorKey).(string); ok {
		return v
	}
	return "system"
}

// --- JSON I/O + маппинг ошибок ---------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeServiceError маппит доменные класс-ошибки в HTTP-статусы.
func (a *API) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		a.log.ErrorContext(r.Context(), "internal error", slog.String("err", err.Error()),
			slog.String("path", r.URL.Path))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// decodeJSON читает тело в v (запрет неизвестных полей -> ловим опечатки клиента).
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
