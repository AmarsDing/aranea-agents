package biz_test

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"
)

func TestA2AUsecase_UpdateAndGetCard(t *testing.T) {
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)
	card, err := uc.UpdateAgentCard(context.Background(), biz.A2AAgentCard{
		AgentID: "agent-1",
		Enabled: true,
		Capabilities: []biz.A2ACapability{
			{Name: "summarize", Description: "Summarize text"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := uc.GetAgentCard(context.Background(), card.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Error("expected card to be enabled")
	}
	if len(got.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(got.Capabilities))
	}
}

func TestA2AUsecase_DisabledByDefault(t *testing.T) {
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)

	_, err := uc.GetAgentCard(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestA2AUsecase_StartInvocation(t *testing.T) {
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)
	inv, err := uc.StartInvocation(context.Background(), biz.A2AInvocation{
		CalleeAgentID: "agent-2",
		Capability:    "summarize",
		PayloadJSON:   `{"text":"hello"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != "pending" {
		t.Errorf("expected status=pending, got %q", inv.Status)
	}
	if inv.ID == "" {
		t.Error("expected non-empty ID")
	}
}
