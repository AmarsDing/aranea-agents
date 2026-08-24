package team

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

func TestResolveRunTokenBudget(t *testing.T) {
	if got := resolveRunTokenBudget(Definition{}); got != DefaultTeamRunInputTokenBudget {
		t.Errorf("zero override = %d, want default %d", got, DefaultTeamRunInputTokenBudget)
	}
	if got := resolveRunTokenBudget(Definition{TokenBudgetInputTokens: 500_000}); got != 500_000 {
		t.Errorf("override = %d, want 500000", got)
	}
	if got := resolveRunTokenBudget(Definition{TokenBudgetInputTokens: -1}); got != -1 {
		t.Errorf("disabled override = %d, want -1", got)
	}
}

func TestAccumulateRunTokenBudget(t *testing.T) {
	r := &Runner{}

	// Ungated run (never registered): always false.
	if tripped, _, _ := r.accumulateRunTokenBudget("run-x", 1_000_000); tripped {
		t.Error("unregistered run must never trip")
	}

	r.registerRunTokenBudget("run-1", 1_000)

	if tripped, used, limit := r.accumulateRunTokenBudget("run-1", 600); tripped || used != 600 || limit != 1_000 {
		t.Errorf("under budget: tripped=%v used=%d limit=%d, want false/600/1000", tripped, used, limit)
	}
	if tripped, used, _ := r.accumulateRunTokenBudget("run-1", 500); !tripped || used != 1_100 {
		t.Errorf("first exceed: tripped=%v used=%d, want true/1100", tripped, used)
	}
	// Second exceed must NOT fire again (single-fire guard).
	if tripped, used, _ := r.accumulateRunTokenBudget("run-1", 500); tripped || used != 1_600 {
		t.Errorf("second exceed: tripped=%v used=%d, want false/1600 (already fired)", tripped, used)
	}

	r.releaseRunTokenBudget("run-1")
	if tripped, _, _ := r.accumulateRunTokenBudget("run-1", 100); tripped {
		t.Error("released run must never trip")
	}

	// Disabled gate (limit<=0) never registers.
	r.registerRunTokenBudget("run-2", -1)
	if tripped, _, _ := r.accumulateRunTokenBudget("run-2", 1<<62); tripped {
		t.Error("disabled gate must never trip")
	}
}

// TestRecordMemberUsage_BudgetTripCancelsRun verifies the wiring from
// recordMemberUsage to RunRegistry.Cancel on budget exceed, and that mirror
// rows (attribution!="") neither accumulate nor trip the gate.
func TestRecordMemberUsage_BudgetTripCancelsRun(t *testing.T) {
	usage := &fakeTeamUsage{}
	sessions := &fakeMetricSessions{}
	r := newUsageTestRunner(usage, sessions)
	r.lg = loggateway.NewNoop()

	reg := rt.NewRunRegistry()
	r.cfg.Runs = reg
	cancelled := make(chan string, 1)
	reg.StoreCancelable("sess-1", "run-1", func() { cancelled <- "cancelled" })

	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	mkAsst := func(tokIn int) biz.ChatMessage {
		return biz.ChatMessage{
			Role: "assistant", Status: biz.TeamMemberStepStatusOK,
			TokenIn: tokIn, TokenOut: 10, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	r.registerRunTokenBudget("run-1", 1_000)

	// Mirror row (attribution set): must not accumulate — a huge value here
	// would otherwise trip immediately.
	r.recordMemberUsage(context.Background(), run, "team-1", ag, mkAsst(900_000), "deepseek", "deepseek-chat", "default", "step-0", 0, "streaming", biz.UsageAttributionMemberLevelStream)
	select {
	case <-cancelled:
		t.Fatal("mirror row must not trip the budget gate")
	case <-time.After(50 * time.Millisecond):
	}

	// Genuine rows accumulate: 700 + 500 = 1200 > 1000 → trip on second row.
	r.recordMemberUsage(context.Background(), run, "team-1", ag, mkAsst(700), "deepseek", "deepseek-chat", "default", "step-1", 0, "streaming", "")
	select {
	case <-cancelled:
		t.Fatal("under-budget row must not cancel the run")
	case <-time.After(50 * time.Millisecond):
	}

	r.recordMemberUsage(context.Background(), run, "team-1", ag, mkAsst(500), "deepseek", "deepseek-chat", "default", "step-2", 0, "streaming", "")
	select {
	case got := <-cancelled:
		if got != "cancelled" {
			t.Fatalf("cancel func fired with %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("budget exceed must cancel the run via RunRegistry")
	}
}
