package service_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// Phase 1c-3: projectionRepo removed — batchSessionRepo now has messages field
// + ActivityLister impl. ListMessagesAfterRevision returns ALL messages (full replay).

func TestSessionProjectionAdapter_GetLatestRevision(t *testing.T) {
	repo := &batchSessionRepo{sessions: map[string]biz.Session{
		"s1": {ID: "s1", SessionRevision: 5},
	}}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop())
	proj := service.NewSessionProjectionAdapter(uc, nil)
	rev, err := proj.GetLatestRevision(context.Background(), "s1")
	if err != nil || rev != 5 {
		t.Fatalf("rev=%d err=%v", rev, err)
	}
}

func TestSessionProjectionAdapter_GetMessagesAfterRevision(t *testing.T) {
	repo := &batchSessionRepo{
		sessions: map[string]biz.Session{
			"s1": {ID: "s1", SessionRevision: 2},
		},
		messages: []biz.ChatMessage{
			{ID: "m1", SessionID: "s1", Role: "user", TurnNumber: 1, TurnID: "t1"},
			{ID: "m2", SessionID: "s1", Role: "assistant", TurnNumber: 2, TurnID: "t2"},
		},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop())
	proj := service.NewSessionProjectionAdapter(uc, nil)
	items, err := proj.GetMessagesAfterRevision(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Phase 1c-3: full replay — all messages returned regardless of revision.
	if len(items) != 2 {
		t.Fatalf("items=%+v", items)
	}
}
