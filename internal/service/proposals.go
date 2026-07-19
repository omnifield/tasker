package service

// Cross-product proposals — a quarantined lane per workspace with a promotion gate.
//
// An architect of product A can drop a proposal into product B's workspace. The proposal is a
// normal node (stable key, activity, priority) but with origin='proposal' and no parent, so it
// is EXCLUDED from B's roadmap roll-up (roots/list filter origin='native') and shows only in
// B's inbox. It enters the roadmap only via an explicit accept, which flips origin to 'native'
// and reparents it where B's architect chose.
//
// Agent-agnostic (same stance as git-flow): tasker enforces the STRUCTURAL gate only — nothing
// a foreign architect drops can silently land in the roadmap; it waits in the inbox until
// accept — and records who proposed it (proposed_by/actor). WHO is allowed to accept/decline is
// a consumer/identity concern (omnifield identity bridge later), not enforced here.

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

// Node origin values. 'native' = a regular roadmap node; 'proposal' = an incoming cross-product
// suggestion awaiting the receiving architect's accept/decline.
const (
	originNative   = "native"
	originProposal = "proposal"
)

// CreateProposalInput — a cross-product proposal dropped into another workspace's inbox.
// SourceWs is the proposer's own workspace key (backlink); Actor is recorded as proposed_by.
type CreateProposalInput struct {
	Title       string
	Description string
	Priority    int64
	SourceWs    string
	Actor       string
}

// CreateProposal creates a quarantined node (origin='proposal', no parent) in the target
// workspace. It allocates a stable key like any node — so the proposer can reference it — but
// stays out of the roadmap until accepted.
func (s *Service) CreateProposal(ctx context.Context, wsIDOrKey string, in CreateProposalInput) (Node, error) {
	if in.Title == "" {
		return Node{}, fmt.Errorf("%w: title required", ErrValidation)
	}
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		seq, err := q.BumpWorkspaceNodeSeq(ctx, store.BumpWorkspaceNodeSeqParams{UpdatedAt: s.nowStr(), ID: ws.ID})
		if err != nil {
			return err
		}
		key := ws.Key + "-" + strconv.FormatInt(seq, 10)
		now := s.nowStr()
		node, err := q.CreateNode(ctx, store.CreateNodeParams{
			ID: s.newID(), WorkspaceID: ws.ID, Seq: seq, Key: key,
			ParentID: sql.NullString{}, Title: in.Title, Description: in.Description,
			StatusID: sql.NullString{}, Priority: in.Priority,
			Origin: originProposal, ProposedBy: in.Actor, SourceWs: in.SourceWs,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, node.ID, in.Actor, "proposed",
			map[string]any{"key": key, "source_ws": in.SourceWs}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, node)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionCreate, "node", out.Key, out)
	return out, nil
}

// ListInbox returns the pending proposals of a workspace (origin='proposal'), enriched.
func (s *Service) ListInbox(ctx context.Context, wsIDOrKey string) ([]Node, error) {
	out := []Node{}
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		ws, err := s.resolveWorkspace(ctx, q, wsIDOrKey)
		if err != nil {
			return err
		}
		nodes, err := q.ListInboxNodes(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			dto, err := s.enrichNode(ctx, q, n)
			if err != nil {
				return err
			}
			out = append(out, dto)
		}
		return nil
	})
	return out, err
}

// AcceptInput — where in the receiving roadmap the accepted proposal lands. ParentID (UUID or
// key, same workspace; nil => roadmap root) and StatusID (same workspace; nil => no status).
type AcceptInput struct {
	ParentID *string
	StatusID *string
	Actor    string
}

// AcceptProposal promotes a proposal into the roadmap: origin -> 'native', with the parent the
// receiving architect chose and a status. The stable key is preserved. Rejects non-proposals.
func (s *Service) AcceptProposal(ctx context.Context, idOrKey string, in AcceptInput) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		if n.Origin != originProposal {
			return fmt.Errorf("%w: node %s is not a proposal", ErrValidation, n.Key)
		}
		parent, err := s.resolveParentInWorkspace(ctx, q, n.WorkspaceID, in.ParentID)
		if err != nil {
			return err
		}
		status, err := s.resolveStatusInWorkspace(ctx, q, n.WorkspaceID, in.StatusID)
		if err != nil {
			return err
		}
		updated, err := q.AcceptProposalNode(ctx, store.AcceptProposalNodeParams{
			ParentID: parent, StatusID: status, UpdatedAt: s.nowStr(), ID: n.ID,
		})
		if err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, updated.ID, in.Actor, "proposal_accepted",
			map[string]any{"from": n.ProposedBy, "source_ws": n.SourceWs}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, updated)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// DeclineInput — optional rationale recorded on the declined proposal.
type DeclineInput struct {
	Comment string
	Actor   string
}

// DeclineProposal rejects a proposal: status -> the workspace's canceled status; origin stays
// 'proposal' so it remains in inbox history, out of the roadmap. Records the rationale.
func (s *Service) DeclineProposal(ctx context.Context, idOrKey string, in DeclineInput) (Node, error) {
	var out Node
	err := s.store.Tx(ctx, func(q *store.Queries) error {
		n, err := s.resolveNode(ctx, q, idOrKey)
		if err != nil {
			return err
		}
		if n.Origin != originProposal {
			return fmt.Errorf("%w: node %s is not a proposal", ErrValidation, n.Key)
		}
		canceled, err := s.workspaceStatusByCategory(ctx, q, n.WorkspaceID, "canceled")
		if err != nil {
			return err
		}
		updated, err := q.DeclineProposalNode(ctx, store.DeclineProposalNodeParams{
			StatusID: canceled, UpdatedAt: s.nowStr(), ID: n.ID,
		})
		if err != nil {
			return err
		}
		if err := s.addActivity(ctx, q, updated.ID, in.Actor, "proposal_declined",
			map[string]any{"comment": in.Comment}); err != nil {
			return err
		}
		out, err = s.enrichNode(ctx, q, updated)
		return err
	})
	if err != nil {
		return Node{}, err
	}
	s.emit(ctx, webhook.ActionUpdate, "node", out.Key, out)
	return out, nil
}

// workspaceStatusByCategory finds the workspace's status of a given category (workspaces seed
// one status per category on creation). Returns a NULL status if none is found (best-effort).
func (s *Service) workspaceStatusByCategory(ctx context.Context, q *store.Queries, wsID, category string) (sql.NullString, error) {
	sts, err := q.ListStatusesByWorkspace(ctx, wsID)
	if err != nil {
		return sql.NullString{}, err
	}
	for _, st := range sts {
		if st.Category == category {
			return sql.NullString{String: st.ID, Valid: true}, nil
		}
	}
	return sql.NullString{}, nil
}
