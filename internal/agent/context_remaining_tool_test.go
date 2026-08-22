package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

func TestGetContextRemaining_NoLedger(t *testing.T) {
	t.Parallel()
	ctx := llmcontext.ContextWithWindow(context.Background(), 1000)
	out, err := getContextRemaining(ctx, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if out.WindowTokens != 1000 || out.Remaining != 1000 {
		t.Fatalf("out = %+v, want window/remaining 1000", out)
	}
	if out.Note == "" {
		t.Fatal("expected note when ledger is missing")
	}
}

func TestGetContextRemaining_WithLedger(t *testing.T) {
	t.Parallel()
	ctx, b := WithContextBudget(llmcontext.ContextWithWindow(context.Background(), 1000))
	b.chars[ContextBudgetCategoryHistory] = 350 // estimateTokens = (700+6)/7 = 100
	out, err := getContextRemaining(ctx, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if out.EstTotalInput != 100 {
		t.Fatalf("est_total_input = %d, want 100", out.EstTotalInput)
	}
	if out.Remaining != 900 {
		t.Fatalf("remaining = %d, want 900", out.Remaining)
	}
}

func TestShouldAttachContextRemaining(t *testing.T) {
	t.Parallel()
	coding := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"}}
	if !shouldAttachContextRemaining(coding, nil) {
		t.Fatal("explicit coding profile must attach")
	}
	deferred := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true}}
	if shouldAttachContextRemaining(deferred, map[string]bool{"datetime": true}) {
		t.Fatal("empty profile + datetime-only face must not attach")
	}
	research := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research", ToolsAllowJSON: `["save_file"]`}}
	if shouldAttachContextRemaining(research, map[string]bool{"save_file": true}) {
		t.Fatal("research profile must not attach")
	}
}
