package a2a

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestCheckCalleeCard(t *testing.T) {
	t.Parallel()
	card := biz.A2AAgentCard{
		AgentID: "a1",
		Enabled: true,
		Capabilities: []biz.A2ACapability{
			{Name: "chat"},
		},
	}
	if err := CheckCalleeCard(card, nil, "chat"); err != nil {
		t.Fatalf("expected pass: %v", err)
	}
	if err := CheckCalleeCard(card, nil, "missing"); err == nil || !strings.Contains(err.Error(), "not advertised") {
		t.Fatalf("expected capability error, got %v", err)
	}
	if err := CheckCalleeCard(biz.A2AAgentCard{Enabled: false}, nil, "chat"); err == nil {
		t.Fatal("expected disabled error")
	}
}
