package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/omnifield/tasker/internal/store"
)

func openTest(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Миграции применились -> таблицы есть, а per-workspace счётчик монотонен и транзакционен.
func TestMigrationsAndSeqCounter(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)

	ws, err := st.CreateWorkspace(ctx, store.CreateWorkspaceParams{
		ID: "ws1", Key: "WS", Name: "n", Description: "", CreatedAt: "t0", UpdatedAt: "t0",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	for want := int64(1); want <= 3; want++ {
		got, err := st.BumpWorkspaceNodeSeq(ctx, store.BumpWorkspaceNodeSeqParams{UpdatedAt: "t1", ID: ws.ID})
		if err != nil {
			t.Fatalf("bump seq: %v", err)
		}
		if got != want {
			t.Fatalf("seq = %d, want %d (counter must be monotonic)", got, want)
		}
	}
}

// dual-id: узел резолвится и по UUID, и по стабильному key.
func TestNodeDualIDResolve(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	_, _ = st.CreateWorkspace(ctx, store.CreateWorkspaceParams{ID: "ws1", Key: "WS", Name: "n", CreatedAt: "t0", UpdatedAt: "t0"})

	n, err := st.CreateNode(ctx, store.CreateNodeParams{
		ID: "node1", WorkspaceID: "ws1", Seq: 1, Key: "WS-1",
		ParentID: sql.NullString{}, Title: "root", CreatedAt: "t0", UpdatedAt: "t0",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	byID, err := st.GetNodeByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	byKey, err := st.GetNodeByKey(ctx, "WS-1")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if byID.ID != byKey.ID {
		t.Fatalf("dual-id resolved different nodes: %s vs %s", byID.ID, byKey.ID)
	}
}

// Транзакция откатывается при ошибке fn (частичная запись не сохраняется).
func TestTxRollback(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)

	sentinel := context.Canceled
	err := st.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{ID: "ws1", Key: "WS", Name: "n", CreatedAt: "t0", UpdatedAt: "t0"}); err != nil {
			return err
		}
		return sentinel // форсим rollback
	})
	if err != sentinel {
		t.Fatalf("Tx err = %v, want sentinel", err)
	}
	if _, err := st.GetWorkspace(ctx, "ws1"); err == nil {
		t.Fatal("workspace persisted despite rollback")
	}
}
