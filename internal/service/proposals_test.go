package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omnifield/tasker/internal/service"
)

// A proposal is quarantined in the target's inbox (not the roadmap) until accept promotes it.
func TestProposalQuarantinedThenAccepted(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "BRAIN", Name: "brainer"})
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "DEV", Name: "devopser"})

	// devopser's architect drops a proposal into brainer's inbox.
	p, err := svc.CreateProposal(ctx, "BRAIN", service.CreateProposalInput{
		Title: "expose agent-session API", SourceWs: "DEV", Actor: "devopser-arch",
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if p.Key != "BRAIN-1" {
		t.Fatalf("key = %q, want BRAIN-1", p.Key)
	}
	if p.Origin != "proposal" || p.ProposedBy != "devopser-arch" || p.SourceWs != "DEV" {
		t.Fatalf("provenance = origin:%q by:%q src:%q", p.Origin, p.ProposedBy, p.SourceWs)
	}

	// Structural gate: NOT in the roadmap tree, but IN the inbox.
	if tree, _ := svc.GetWorkspaceTree(ctx, "BRAIN", -1); len(tree) != 0 {
		t.Fatalf("roadmap must be empty (proposal quarantined), got %d roots", len(tree))
	}
	inbox, _ := svc.ListInbox(ctx, "BRAIN")
	if len(inbox) != 1 || inbox[0].Key != p.Key {
		t.Fatalf("inbox = %+v, want [%s]", inbox, p.Key)
	}

	// brainer's architect accepts into the roadmap with a status.
	todo := statusID(t, svc, "BRAIN", "todo")
	acc, err := svc.AcceptProposal(ctx, p.Key, service.AcceptInput{StatusID: &todo, Actor: "brain-arch"})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if acc.Origin != "native" {
		t.Fatalf("origin after accept = %q, want native", acc.Origin)
	}
	if acc.Key != p.Key {
		t.Fatalf("stable key changed on accept: %q != %q", acc.Key, p.Key)
	}

	// Now in the roadmap, gone from the inbox.
	tree, _ := svc.GetWorkspaceTree(ctx, "BRAIN", -1)
	if len(tree) != 1 || tree[0].Key != p.Key {
		t.Fatalf("roadmap must contain the accepted node, got %+v", tree)
	}
	if inbox, _ := svc.ListInbox(ctx, "BRAIN"); len(inbox) != 0 {
		t.Fatalf("inbox must be empty after accept, got %d", len(inbox))
	}
}

// Accept can reparent the proposal under an existing roadmap node of the target workspace.
func TestProposalAcceptUnderParent(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "BRAIN", Name: "brainer"})
	root, _ := svc.CreateNode(ctx, "BRAIN", service.CreateNodeInput{Title: "roadmap root"})
	p, _ := svc.CreateProposal(ctx, "BRAIN", service.CreateProposalInput{Title: "idea", Actor: "a"})

	acc, err := svc.AcceptProposal(ctx, p.Key, service.AcceptInput{ParentID: &root.Key, Actor: "b"})
	if err != nil {
		t.Fatalf("accept under parent: %v", err)
	}
	if acc.ParentID == nil || *acc.ParentID != root.ID {
		t.Fatalf("accepted parent = %v, want %s", acc.ParentID, root.ID)
	}
}

// Decline keeps the proposal out of the roadmap and marks it canceled (origin stays proposal).
func TestProposalDeclineStaysOutOfRoadmap(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "BRAIN", Name: "brainer"})
	p, _ := svc.CreateProposal(ctx, "BRAIN", service.CreateProposalInput{Title: "x", Actor: "a"})

	d, err := svc.DeclineProposal(ctx, p.Key, service.DeclineInput{Comment: "out of scope", Actor: "brain-arch"})
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if d.Origin != "proposal" {
		t.Fatalf("origin after decline = %q, want proposal (kept for history)", d.Origin)
	}
	canceled := statusID(t, svc, "BRAIN", "canceled")
	if d.StatusID == nil || *d.StatusID != canceled {
		t.Fatalf("declined status = %v, want canceled %s", d.StatusID, canceled)
	}
	if tree, _ := svc.GetWorkspaceTree(ctx, "BRAIN", -1); len(tree) != 0 {
		t.Fatalf("declined proposal must not enter roadmap, got %d roots", len(tree))
	}
}

// accept/decline reject nodes that are not proposals (structural guard).
func TestAcceptDeclineRejectNativeNode(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	_, _ = svc.CreateWorkspace(ctx, service.CreateWorkspaceInput{Key: "TASK", Name: "T"})
	n, _ := svc.CreateNode(ctx, "TASK", service.CreateNodeInput{Title: "native"})
	if n.Origin != "native" {
		t.Fatalf("regular node origin = %q, want native", n.Origin)
	}
	if _, err := svc.AcceptProposal(ctx, n.Key, service.AcceptInput{}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("accept native err = %v, want ErrValidation", err)
	}
	if _, err := svc.DeclineProposal(ctx, n.Key, service.DeclineInput{}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("decline native err = %v, want ErrValidation", err)
	}
}
