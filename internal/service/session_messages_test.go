package service_test

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// Phase 1c-3: afterRevisionSessionRepo removed — batchSessionRepo now has
// messages field + ActivityLister impl. ListMessagesAfterRevision returns ALL
// messages (full replay); the revision parameter is kept for WS sync signaling.

func TestSessionService_ListSessionMessages_afterRevision(t *testing.T) {
	repo := &batchSessionRepo{
		sessions: map[string]biz.Session{
			"s1": {ID: "s1", SessionRevision: 2},
		},
		messages: []biz.ChatMessage{
			{ID: "u1", SessionID: "s1", Role: "user", TurnNumber: 1, TurnID: "t1"},
			{ID: "a1", SessionID: "s1", Role: "assistant", TurnNumber: 1, TurnID: "t1"},
			{ID: "u2", SessionID: "s1", Role: "user", TurnNumber: 2, TurnID: "t2"},
			{ID: "a2", SessionID: "s1", Role: "assistant", TurnNumber: 2, TurnID: "t2"},
		},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop())
	svc := service.NewSessionService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	after := int64(1)
	resp, err := svc.ListSessionMessages(context.Background(), &v1.ListSessionMessagesRequest{
		Id:            "s1",
		AfterRevision: &after,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.GetCurrentRevision() != 2 {
		t.Fatalf("current_revision: got %d want 2", resp.GetCurrentRevision())
	}
	// Phase 1c-3: full replay — all messages returned regardless of revision.
	// The revision parameter is kept for WS sync signaling; client dedups by ID.
	if len(resp.GetItems()) != 4 {
		t.Fatalf("items: got %d want 4 (full replay)", len(resp.GetItems()))
	}
	if resp.GetItems()[0].GetId() != "u1" || resp.GetItems()[3].GetId() != "a2" {
		t.Fatalf("unexpected items: %+v", resp.GetItems())
	}

	afterLast := int64(2)
	resp2, err := svc.ListSessionMessages(context.Background(), &v1.ListSessionMessagesRequest{
		Id:            "s1",
		AfterRevision: &afterLast,
	})
	if err != nil {
		t.Fatalf("list last: %v", err)
	}
	// Full replay: all messages returned regardless of revision value.
	if len(resp2.GetItems()) != 4 {
		t.Fatalf("after last revision: got %d items want 4 (full replay)", len(resp2.GetItems()))
	}
}
