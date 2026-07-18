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

func TestDependsOnDirectionAndEnrichedCrossWS(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "PROD", Name: "Product"})
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "INFRA", Name: "Infra"})
	prod, _ := svc.CreateNode(ctx, "PROD", service.CreateNodeInput{Title: "roadmap item"})
	infra, _ := svc.CreateNode(ctx, "INFRA", service.CreateNodeInput{Title: "universal-bridge"})

	// PROD-узел зависит от чужого INFRA-узла (направленное depends_on: from=prod -> to=infra).
	if _, err := svc.AddRelation(ctx, prod.Key, service.CreateRelationInput{ToNode: infra.Key, Kind: "depends_on"}); err != nil {
		t.Fatalf("depends_on cross-ws: %v", err)
	}

	// со стороны PROD: outgoing depends_on на чужой INFRA-узел, обогащённый ws/key/title.
	relsProd, err := svc.ListRelations(ctx, prod.Key)
	if err != nil {
		t.Fatalf("list prod: %v", err)
	}
	if len(relsProd) != 1 {
		t.Fatalf("prod relations = %d, want 1", len(relsProd))
	}
	rp := relsProd[0]
	if rp.Kind != "depends_on" || rp.Direction != "outgoing" || !rp.CrossWorkspace {
		t.Fatalf("prod view = %+v, want depends_on/outgoing/cross", rp)
	}
	if rp.Other.Key != infra.Key || rp.Other.WorkspaceKey != "INFRA" || rp.Other.Title != "universal-bridge" {
		t.Fatalf("prod.other = %+v, want %s/INFRA/universal-bridge", rp.Other, infra.Key)
	}

	// со стороны INFRA: та же связь видна как incoming (кто-то зависит от него).
	relsInfra, _ := svc.ListRelations(ctx, infra.Key)
	if len(relsInfra) != 1 || relsInfra[0].Direction != "incoming" || relsInfra[0].Other.WorkspaceKey != "PROD" {
		t.Fatalf("infra view = %+v, want 1 incoming from PROD", relsInfra)
	}
}

func TestSubtreeFetch(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	root, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "root"})
	child, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "child", ParentID: &root.Key})
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "grandchild", ParentID: &child.Key})

	// Полное поддерево (без лимита): root -> child -> grandchild.
	tree, err := svc.GetSubtree(ctx, root.Key, -1)
	if err != nil {
		t.Fatalf("subtree: %v", err)
	}
	if tree.Key != root.Key || len(tree.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(tree.Children))
	}
	if len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].Title != "grandchild" {
		t.Fatalf("grandchild missing under child: %+v", tree.Children[0])
	}
	// Обогащение доходит до листьев (rollup присутствует).
	if tree.Rollup.Total != 1 {
		t.Fatalf("root rollup.total = %d, want 1", tree.Rollup.Total)
	}

	// depth=1 — только прямые дети, без внуков.
	shallow, _ := svc.GetSubtree(ctx, root.Key, 1)
	if len(shallow.Children) != 1 || len(shallow.Children[0].Children) != 0 {
		t.Fatalf("depth=1 leaked grandchildren: %+v", shallow.Children)
	}
	// depth=0 — только сам узел.
	just, _ := svc.GetSubtree(ctx, root.Key, 0)
	if len(just.Children) != 0 {
		t.Fatalf("depth=0 children = %d, want 0", len(just.Children))
	}

	// workspace-tree — один корень с поддеревом.
	roots, _ := svc.GetWorkspaceTree(ctx, "TASK", -1)
	if len(roots) != 1 || roots[0].Key != root.Key || len(roots[0].Children) != 1 {
		t.Fatalf("ws tree = %+v, want [root->child]", roots)
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

func TestChildrenOrderedByPriority(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	root, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "root"})

	// Создаём в порядке приоритетов 3,0,1 — ожидаем выдачу по возрастанию (P0 первым).
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "p3", ParentID: &root.Key, Priority: 3})
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "p0", ParentID: &root.Key, Priority: 0})
	_, _ = svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "p1", ParentID: &root.Key, Priority: 1})

	kids, err := svc.ListChildren(ctx, root.Key)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	got := []string{kids[0].Title, kids[1].Title, kids[2].Title}
	want := []string{"p0", "p1", "p3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children order = %v, want %v (priority asc)", got, want)
		}
	}
	// В поддереве порядок тот же.
	tree, _ := svc.GetSubtree(ctx, root.Key, -1)
	if tree.Children[0].Title != "p0" || tree.Children[2].Title != "p3" {
		t.Fatalf("subtree order = %v, want p0..p3", []string{tree.Children[0].Title, tree.Children[1].Title, tree.Children[2].Title})
	}
}

func TestDeleteNodeCleansDependents(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})

	// Минимальный репро: голый лист без явных зависимостей. У него всё равно есть
	// activity=created — раньше это роняло удаление на FOREIGN KEY constraint (500).
	leaf, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "leaf", Actor: "egor"})
	if err := svc.DeleteNode(ctx, leaf.Key); err != nil {
		t.Fatalf("delete bare leaf: %v", err)
	}
	if _, err := svc.GetNode(ctx, leaf.Key); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("get deleted leaf err = %v, want ErrNotFound", err)
	}

	// Узел со всеми зависимостями: метка, ассайни, комментарий, связь на соседа.
	peer, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "peer"})
	n, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "rich", Actor: "egor"})
	lbl, _ := svc.CreateLabel(ctx, "TASK", service.CreateLabelInput{Name: "frontend", Color: "#8e44ad"})
	if _, err := svc.AddNodeLabel(ctx, n.Key, lbl.ID); err != nil {
		t.Fatalf("attach label: %v", err)
	}
	if _, err := svc.AddAssignee(ctx, n.Key, "claude", "egor"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if _, err := svc.AddRelation(ctx, n.Key, service.CreateRelationInput{ToNode: peer.Key, Kind: "blocks"}); err != nil {
		t.Fatalf("relation: %v", err)
	}
	if _, err := svc.AddActivity(ctx, n.Key, service.AddActivityInput{Actor: "egor", Data: []byte(`{"text":"note"}`)}); err != nil {
		t.Fatalf("comment: %v", err)
	}

	if err := svc.DeleteNode(ctx, n.Key); err != nil {
		t.Fatalf("delete node with dependents: %v", err)
	}
	// Сосед цел, но связь на удалённый узел исчезла (почищена с обеих сторон).
	if _, err := svc.GetNode(ctx, peer.Key); err != nil {
		t.Fatalf("peer must survive: %v", err)
	}
	rels, _ := svc.ListRelations(ctx, peer.Key)
	if len(rels) != 0 {
		t.Fatalf("peer relations = %d, want 0 (dangling edge cleaned)", len(rels))
	}
}
