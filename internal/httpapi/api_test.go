package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/omnifield/tasker/internal/httpapi"
	"github.com/omnifield/tasker/internal/service"
	"github.com/omnifield/tasker/internal/store"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := httptest.NewServer(httpapi.New(service.New(st, nil), nil).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// do — авторизованный запрос (token-stub). Возвращает статус и распарсенное тело.
func do(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rdr)
	req.Header.Set("Authorization", "Bearer egor")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func TestHealthzNoAuth(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/tasker/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/tasker/workspaces")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}
}

func TestWorkspaceNodeFlowOverHTTP(t *testing.T) {
	srv := newServer(t)

	if st, _ := do(t, srv, "POST", "/tasker/workspaces", map[string]any{"key": "task", "name": "T"}); st != http.StatusCreated {
		t.Fatalf("create ws = %d, want 201", st)
	}
	st, root := do(t, srv, "POST", "/tasker/workspaces/TASK/nodes", map[string]any{"title": "root"})
	if st != http.StatusCreated {
		t.Fatalf("create node = %d, want 201", st)
	}
	if root["key"] != "TASK-1" {
		t.Fatalf("key = %v, want TASK-1", root["key"])
	}

	// dual-id по key и по UUID резолвят один узел.
	_, byKey := do(t, srv, "GET", "/tasker/nodes/TASK-1", nil)
	_, byID := do(t, srv, "GET", "/tasker/nodes/"+root["id"].(string), nil)
	if byKey["id"] != byID["id"] {
		t.Fatal("dual-id mismatch over HTTP")
	}

	// rollup присутствует в ответе (derived).
	if _, ok := byKey["rollup"].(map[string]any); !ok {
		t.Fatalf("node response has no rollup: %v", byKey)
	}

	// missing node -> 404.
	if st, _ := do(t, srv, "GET", "/tasker/nodes/TASK-404", nil); st != http.StatusNotFound {
		t.Fatalf("missing node = %d, want 404", st)
	}
	// пустой title -> 400.
	if st, _ := do(t, srv, "POST", "/tasker/workspaces/TASK/nodes", map[string]any{"title": ""}); st != http.StatusBadRequest {
		t.Fatalf("empty title = %d, want 400", st)
	}
	// неизвестное поле -> 400 (DisallowUnknownFields).
	if st, _ := do(t, srv, "POST", "/tasker/workspaces/TASK/nodes", map[string]any{"title": "x", "bogus": 1}); st != http.StatusBadRequest {
		t.Fatalf("unknown field = %d, want 400", st)
	}
}

func TestFilterNodesByParent(t *testing.T) {
	srv := newServer(t)
	_, _ = do(t, srv, "POST", "/tasker/workspaces", map[string]any{"key": "task", "name": "T"})
	_, root := do(t, srv, "POST", "/tasker/workspaces/TASK/nodes", map[string]any{"title": "root"})
	_, _ = do(t, srv, "POST", "/tasker/workspaces/TASK/nodes", map[string]any{"title": "child", "parent_id": root["key"]})

	// ?parent=none -> только корни (1).
	req, _ := http.NewRequest("GET", srv.URL+"/tasker/workspaces/TASK/nodes?parent=none", nil)
	req.Header.Set("Authorization", "Bearer egor")
	resp, _ := srv.Client().Do(req)
	defer func() { _ = resp.Body.Close() }()
	var roots []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&roots)
	if len(roots) != 1 || roots[0]["key"] != "TASK-1" {
		t.Fatalf("parent=none returned %v, want [TASK-1]", roots)
	}
}
