package service_test

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

type afterRevisionSessionRepo struct {
	batchSessionRepo
	messages []biz.ChatMessage
}

func (m *afterRevisionSessionRepo) ListMessagesAfterRevision(_ context.Context, _ string, afterRevision int64) ([]biz.ChatMessage, error) {
	if afterRevision <= 0 {
		return m.messages, nil
	}
	var out []biz.ChatMessage
	for _, msg := range m.messages {
		if int64(msg.TurnIndex) > afterRevision*2 {
			out = append(out, msg)
		}
	}
	return out, nil
}

func TestSessionService_ListSessionMessages_afterRevision(t *testing.T) {
	repo := &afterRevisionSessionRepo{
		batchSessionRepo: batchSessionRepo{sessions: map[string]biz.Session{
			"s1": {ID: "s1", SessionRevision: 2},
		}},
		messages: []biz.ChatMessage{
			{ID: "u1", SessionID: "s1", Role: "user", TurnIndex: 1},
			{ID: "a1", SessionID: "s1", Role: "assistant", TurnIndex: 2},
			{ID: "u2", SessionID: "s1", Role: "user", TurnIndex: 3},
			{ID: "a2", SessionID: "s1", Role: "assistant", TurnIndex: 4},
		},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil)
	svc := service.NewSessionService(uc, nil)

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
	if len(resp.GetItems()) != 2 {
		t.Fatalf("items: got %d want 2 (turn 2 only)", len(resp.GetItems()))
	}
	if resp.GetItems()[0].GetId() != "u2" || resp.GetItems()[1].GetId() != "a2" {
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
	if len(resp2.GetItems()) != 0 {
		t.Fatalf("after last revision: got %d items want 0", len(resp2.GetItems()))
	}
}
