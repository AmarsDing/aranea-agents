package service_test

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// TestSessionService_GetSessionTree verifies the GetSessionTree RPC handler
// returns the recursive session tree shape produced by biz.SessionUsecase.
// Phase B-1: replaces N+1 ListChildSessions recursion with one query.
func TestSessionService_GetSessionTree(t *testing.T) {
	// Build a tiny recursive tree: spirit root + 1 team child + 1 agent grandchild.
	tree := &biz.SessionTree{
		Root: biz.Session{ID: "spirit-1", Title: "Spirit Root", OwnerType: "spirit"},
		Children: []*biz.SessionTreeNode{
			{
				Session: biz.Session{ID: "team-1", Title: "Team 1", ParentSessionID: "spirit-1", OwnerType: "team"},
				Children: []*biz.SessionTreeNode{
					{Session: biz.Session{ID: "agent-1", Title: "Agent A", ParentSessionID: "team-1", OwnerType: "agent"}},
				},
			},
		},
	}
	repo := &batchSessionRepo{
		sessions: map[string]biz.Session{"spirit-1": tree.Root},
		tree:     tree,
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc := service.NewSessionService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.GetSessionTree(context.Background(), &v1.GetSessionTreeRequest{SpiritSessionId: "spirit-1"})
	if err != nil {
		t.Fatalf("GetSessionTree: %v", err)
	}
	if resp.Root == nil {
		t.Fatalf("root is nil")
	}
	if resp.Root.Session == nil || resp.Root.Session.Id != "spirit-1" {
		t.Fatalf("root session mismatch: %+v", resp.Root.Session)
	}
	if len(resp.Root.Children) != 1 {
		t.Fatalf("children len: got %d want 1", len(resp.Root.Children))
	}
	team := resp.Root.Children[0]
	if team.Session == nil || team.Session.Id != "team-1" {
		t.Fatalf("team child mismatch: %+v", team.Session)
	}
	if len(team.Children) != 1 || team.Children[0].Session == nil || team.Children[0].Session.Id != "agent-1" {
		t.Fatalf("grandchild mismatch: %+v", team.Children)
	}
}

// TestSessionService_GetSessionTree_validation verifies empty spirit_session_id
// is rejected with a bad request error (Phase B-1).
func TestSessionService_GetSessionTree_validation(t *testing.T) {
	repo := &batchSessionRepo{sessions: map[string]biz.Session{}}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc := service.NewSessionService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := svc.GetSessionTree(context.Background(), &v1.GetSessionTreeRequest{SpiritSessionId: ""})
	if err == nil {
		t.Fatal("expected validation error for empty spirit_session_id")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected bad request, got %v", err)
	}
}
