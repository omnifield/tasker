package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/omnifield/tasker/internal/service"
)

// --- workspaces ------------------------------------------------------------

func (a *API) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := a.svc.ListWorkspaces(r.Context())
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (a *API) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws, err := a.svc.CreateWorkspace(r.Context(), service.CreateWorkspaceInput{
		Key: body.Key, Name: body.Name, Description: body.Description, Actor: actorOf(r),
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (a *API) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := a.svc.GetWorkspace(r.Context(), r.PathValue("ws"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (a *API) handleListStatuses(w http.ResponseWriter, r *http.Request) {
	sts, err := a.svc.ListStatuses(r.Context(), r.PathValue("ws"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sts)
}

func (a *API) handleCreateStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Category string `json:"category"`
		Name     string `json:"name"`
		Color    string `json:"color"`
		Position int64  `json:"position"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := a.svc.CreateStatus(r.Context(), r.PathValue("ws"), service.CreateStatusInput{
		Category: body.Category, Name: body.Name, Color: body.Color, Position: body.Position,
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

// --- nodes -----------------------------------------------------------------

func (a *API) handleListNodes(w http.ResponseWriter, r *http.Request) {
	var f service.NodeFilter
	q := r.URL.Query()
	// ?parent=<idOrKey|none> — none/root возвращает только корни; отсутствие параметра — все.
	if q.Has("parent") {
		f.ParentSet = true
		if v := q.Get("parent"); v != "" && v != "none" && v != "root" && v != "null" {
			f.ParentID = &v
		}
	}
	if q.Has("status") {
		f.StatusSet = true
		if v := q.Get("status"); v != "" && v != "none" && v != "null" {
			f.StatusID = &v
		}
	}
	// parent-фильтр по key/UUID: резолвим в UUID до in-memory сравнения (совпадает с n.ParentID).
	if f.ParentID != nil {
		n, err := a.svc.GetNode(r.Context(), *f.ParentID)
		if err != nil {
			a.writeServiceError(w, r, err)
			return
		}
		f.ParentID = &n.ID
	}
	nodes, err := a.svc.ListNodes(r.Context(), r.PathValue("ws"), f)
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (a *API) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		ParentID    *string `json:"parent_id"`
		StatusID    *string `json:"status_id"`
		Priority    int64   `json:"priority"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := a.svc.CreateNode(r.Context(), r.PathValue("ws"), service.CreateNodeInput{
		Title: body.Title, Description: body.Description, ParentID: body.ParentID,
		StatusID: body.StatusID, Priority: body.Priority, Actor: actorOf(r),
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (a *API) handleGetNode(w http.ResponseWriter, r *http.Request) {
	n, err := a.svc.GetNode(r.Context(), r.PathValue("key"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	// Частичный PATCH: читаем в map, чтобы отличить «поле не прислано» от «null» (очистка).
	var raw map[string]json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in := service.UpdateNodeInput{Actor: actorOf(r)}
	if v, ok := raw["title"]; ok {
		s, err := asString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "title must be a string")
			return
		}
		in.Title = &s
	}
	if v, ok := raw["description"]; ok {
		s, err := asString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "description must be a string")
			return
		}
		in.Description = &s
	}
	if v, ok := raw["priority"]; ok {
		var p int64
		if err := json.Unmarshal(v, &p); err != nil {
			writeError(w, http.StatusBadRequest, "priority must be a number")
			return
		}
		in.Priority = &p
	}
	if v, ok := raw["status_id"]; ok {
		in.SetStatus = true
		in.StatusID = asNullableString(v)
	}
	if v, ok := raw["parent_id"]; ok {
		in.SetParent = true
		in.ParentID = asNullableString(v)
	}
	n, err := a.svc.UpdateNode(r.Context(), r.PathValue("key"), in)
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteNode(r.Context(), r.PathValue("key")); err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListChildren(w http.ResponseWriter, r *http.Request) {
	kids, err := a.svc.ListChildren(r.Context(), r.PathValue("key"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, kids)
}

// --- relations -------------------------------------------------------------

func (a *API) handleListRelations(w http.ResponseWriter, r *http.Request) {
	rels, err := a.svc.ListRelations(r.Context(), r.PathValue("key"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

func (a *API) handleCreateRelation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ToNode string `json:"to_node"`
		Kind   string `json:"kind"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rel, err := a.svc.AddRelation(r.Context(), r.PathValue("key"), service.CreateRelationInput{
		ToNode: body.ToNode, Kind: body.Kind, Actor: actorOf(r),
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

func (a *API) handleDeleteRelation(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteRelation(r.Context(), r.PathValue("id")); err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- activity --------------------------------------------------------------

func (a *API) handleListActivity(w http.ResponseWriter, r *http.Request) {
	acts, err := a.svc.ListActivity(r.Context(), r.PathValue("key"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, acts)
}

func (a *API) handleCreateActivity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	act, err := a.svc.AddActivity(r.Context(), r.PathValue("key"), service.AddActivityInput{
		Kind: body.Kind, Data: body.Data, Actor: actorOf(r),
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, act)
}

// --- labels / assignees ----------------------------------------------------

func (a *API) handleListLabels(w http.ResponseWriter, r *http.Request) {
	ls, err := a.svc.ListLabels(r.Context(), r.PathValue("ws"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

func (a *API) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	l, err := a.svc.CreateLabel(r.Context(), r.PathValue("ws"), service.CreateLabelInput{
		Name: body.Name, Color: body.Color,
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *API) handleAttachLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LabelID string `json:"label_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := a.svc.AddNodeLabel(r.Context(), r.PathValue("key"), body.LabelID)
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) handleDetachLabel(w http.ResponseWriter, r *http.Request) {
	n, err := a.svc.RemoveNodeLabel(r.Context(), r.PathValue("key"), r.PathValue("label_id"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) handleAddAssignee(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Assignee string `json:"assignee"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := a.svc.AddAssignee(r.Context(), r.PathValue("key"), body.Assignee, actorOf(r))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) handleRemoveAssignee(w http.ResponseWriter, r *http.Request) {
	n, err := a.svc.RemoveAssignee(r.Context(), r.PathValue("key"), r.PathValue("actor"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// --- JSON-хелперы частичного PATCH -----------------------------------------

// asString извлекает Go-строку из JSON-строки.
func asString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

// asNullableString: JSON null -> nil (очистка поля); JSON-строка -> *string. Прочее -> nil.
func asNullableString(raw json.RawMessage) *string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	return &s
}
