package service_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

type projectionRepo struct {
	batchSessionRepo
	messages []biz.ChatMessage
}

func (p *projectionRepo) ListMessagesAfterRevision(_ context.Context, _ string, afterRevision int64) ([]biz.ChatMessage, error) {
	var out []biz.ChatMessage
	for _, m := range p.messages {
		if int64(m.TurnNumber) > afterRevision {
			out = append(out, m)
		}
	}
	return out, nil
}

func TestSessionProjectionAdapter_GetLatestRevision(t *testing.T) {
	repo := &projectionRepo{
		batchSessionRepo: batchSessionRepo{sessions: map[string]biz.Session{
			"s1": {ID: "s1", SessionRevision: 5},
		}},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil)
	proj := service.NewSessionProjectionAdapter(uc, nil)
	rev, err := proj.GetLatestRevision(context.Background(), "s1")
	if err != nil || rev != 5 {
		t.Fatalf("rev=%d err=%v", rev, err)
	}
}

func TestSessionProjectionAdapter_GetMessagesAfterRevision(t *testing.T) {
	repo := &projectionRepo{
		batchSessionRepo: batchSessionRepo{sessions: map[string]biz.Session{
			"s1": {ID: "s1", SessionRevision: 2},
		}},
		messages: []biz.ChatMessage{
			{ID: "m1", TurnNumber: 1, TurnID: "t1"},
			{ID: "m2", TurnNumber: 2, TurnID: "t2"},
		},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil)
	proj := service.NewSessionProjectionAdapter(uc, nil)
	items, err := proj.GetMessagesAfterRevision(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "m2" {
		t.Fatalf("items=%+v", items)
	}
}
