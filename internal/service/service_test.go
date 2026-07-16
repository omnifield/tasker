package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/omnifield/tasker/internal/service"
	"github.com/omnifield/tasker/internal/store"
)

func newSvc(t *testing.T) *service.Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return service.New(st, nil) // nil hook -> webhook.Nop
}

func statusID(t *testing.T, svc *service.Service, ws, category string) string {
	t.Helper()
	sts, err := svc.ListStatuses(context.Background(), ws)
	if err != nil {
		t.Fatalf("list statuses: %v", err)
	}
	for _, s := range sts {
		if s.Category == category {
			return s.ID
		}
	}
	t.Fatalf("no seeded status for category %q", category)
	return ""
}

func TestWorkspaceKeyNormalizationAndConflict(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	ws, err := svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "task", Name: "Tasker"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ws.Key != "TASK" {
		t.Fatalf("key = %q, want normalized TASK", ws.Key)
	}
	if _, err := svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "dup"}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("dup key err = %v, want ErrConflict", err)
	}
	if _, err := svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "bad-key", Name: "x"}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("bad key err = %v, want ErrValidation", err)
	}
}

func TestNodeKeyStableAndDualID(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})

	n1, err := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "a"})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if n1.Key != "TASK-1" {
		t.Fatalf("key = %q, want TASK-1", n1.Key)
	}
	n2, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "b"})
	if n2.Key != "TASK-2" {
		t.Fatalf("key = %q, want TASK-2 (monotonic)", n2.Key)
	}

	byKey, err := svc.GetNode(ctx, "TASK-1")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	byID, err := svc.GetNode(ctx, n1.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if byKey.ID != byID.ID {
		t.Fatal("dual-id resolved different nodes")
	}
}

func TestRollupDerived(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	done := statusID(t, svc, "TASK", "done")
	inprog := statusID(t, svc, "TASK", "in_progress")

	root, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "root"})

	// Нет детей -> отражает собственную категорию (backlog по умолчанию).
	if root.Rollup.HasChildren || root.Rollup.Category != "backlog" {
		t.Fatalf("childless rollup = %+v, want backlog/no-children", root.Rollup)
	}

	kd := done
	ki := inprog
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "c1", ParentID: &root.Key, StatusID: &kd})
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "c2", ParentID: &root.Key, StatusID: &ki})

	got, _ := svc.GetNode(ctx, root.Key)
	r := got.Rollup
	if !r.HasChildren || r.Total != 2 || r.Completed != 1 || r.Fraction != 0.5 || r.Category != "in_progress" {
		t.Fatalf("rollup = %+v, want total2/completed1/0.5/in_progress", r)
	}

	// Все дети done -> свёртка done, fraction 1.0.
	_, _ = svc.UpdateNode(ctx, "TASK-3", service.UpdateNodeInput{SetStatus: true, StatusID: &kd})
	got, _ = svc.GetNode(ctx, root.Key)
	if got.Rollup.Category != "done" || got.Rollup.Fraction != 1.0 {
		t.Fatalf("all-done rollup = %+v, want done/1.0", got.Rollup)
	}
}

func TestFKEnforcementAppLevel(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "AAA", Name: "A"})
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "BBB", Name: "B"})
	aNode, _ := svc.CreateNode(ctx, "AAA", service.CreateNodeInput{Title: "a"})

	// parent из другого workspace -> ErrValidation (дерево не пересекает границы ws).
	if _, err := svc.CreateNode(ctx, "BBB", service.CreateNodeInput{Title: "x", ParentID: &aNode.Key}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("cross-ws parent err = %v, want ErrValidation", err)
	}
	// несуществующий parent -> ErrNotFound.
	ghost := "AAA-999"
	if _, err := svc.CreateNode(ctx, "AAA", service.CreateNodeInput{Title: "x", ParentID: &ghost}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("ghost parent err = %v, want ErrNotFound", err)
	}
	// status из другого workspace -> ErrValidation.
	bStatus := statusID(t, svc, "BBB", "todo")
	if _, err := svc.CreateNode(ctx, "AAA", service.CreateNodeInput{Title: "x", StatusID: &bStatus}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("cross-ws status err = %v, want ErrValidation", err)
	}
}

func TestReparentCycleGuard(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	root, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "root"})
	child, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "child", ParentID: &root.Key})

	// root.parent = child -> цикл.
	if _, err := svc.UpdateNode(ctx, root.Key, service.UpdateNodeInput{SetParent: true, ParentID: &child.Key}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("cycle err = %v, want ErrValidation", err)
	}
	// self-parent -> тоже отказ.
	if _, err := svc.UpdateNode(ctx, root.Key, service.UpdateNodeInput{SetParent: true, ParentID: &root.Key}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("self-parent err = %v, want ErrValidation", err)
	}
}

func TestRelationsTypedAndCrossWorkspace(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "AAA", Name: "A"})
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "BBB", Name: "B"})
	a, _ := svc.CreateNode(ctx, "AAA", service.CreateNodeInput{Title: "a"})
	b, _ := svc.CreateNode(ctx, "BBB", service.CreateNodeInput{Title: "b"})

	// cross-workspace relation допустим.
	rel, err := svc.AddRelation(ctx, a.Key, service.CreateRelationInput{ToNode: b.Key, Kind: "blocks"})
	if err != nil {
		t.Fatalf("cross-ws relation: %v", err)
	}
	if rel.Kind != "blocks" {
		t.Fatalf("kind = %q", rel.Kind)
	}
	// невалидный kind -> ErrValidation.
	if _, err := svc.AddRelation(ctx, a.Key, service.CreateRelationInput{ToNode: b.Key, Kind: "parent"}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("bad kind err = %v, want ErrValidation", err)
	}
	// self-link -> ErrValidation.
	if _, err := svc.AddRelation(ctx, a.Key, service.CreateRelationInput{ToNode: a.Key, Kind: "relates"}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("self-link err = %v, want ErrValidation", err)
	}
	// связь видна с обоих концов.
	relsB, _ := svc.ListRelations(ctx, b.Key)
	if len(relsB) != 1 {
		t.Fatalf("target sees %d relations, want 1", len(relsB))
	}
}

func TestActivityTimeline(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	done := statusID(t, svc, "TASK", "done")
	n, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "n", Actor: "egor"})
	_, _ = svc.UpdateNode(ctx, n.Key, service.UpdateNodeInput{SetStatus: true, StatusID: &done, Actor: "egor"})
	_, _ = svc.AddActivity(ctx, n.Key, service.AddActivityInput{Actor: "egor", Data: []byte(`{"text":"hi"}`)})

	acts, err := svc.ListActivity(ctx, n.Key)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	want := []string{"created", "status_changed", "commented"}
	if len(acts) != len(want) {
		t.Fatalf("activity count = %d, want %d", len(acts), len(want))
	}
	for i, k := range want {
		if acts[i].Kind != k {
			t.Fatalf("activity[%d].kind = %q, want %q", i, acts[i].Kind, k)
		}
	}
}

func TestDeleteNodeWithChildrenConflict(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	root, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "root"})
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "child", ParentID: &root.Key})

	if err := svc.DeleteNode(ctx, root.Key); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("delete-with-children err = %v, want ErrConflict", err)
	}
}
