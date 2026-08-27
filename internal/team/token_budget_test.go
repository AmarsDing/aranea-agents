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

// TestAccumulateRunTokenBudgetFromStream verifies the mid-stream gate entry
// (2026-08-26 M80 fix): usage deltas fed via the stream-consumer hook
// accumulate against the armed run budget, the first exceed cancels the run
// exactly once, and later deltas never re-fire.
func TestAccumulateRunTokenBudgetFromStream(t *testing.T) {
	r := &Runner{lg: loggateway.NewNoop()}
	reg := rt.NewRunRegistry()
	r.cfg.Runs = reg
	cancelled := make(chan string, 1)
	reg.StoreCancelable("sess-1", "run-1", func() { cancelled <- "cancelled" })

	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	r.registerRunTokenBudget("run-1", 1_000)

	// Under budget: no cancel, tripped=false.
	if tripped := r.accumulateRunTokenBudgetFromStream(context.Background(), run, "team-1", 600); tripped {
		t.Fatal("under-budget delta must report tripped=false")
	}
	select {
	case <-cancelled:
		t.Fatal("under-budget delta must not cancel the run")
	case <-time.After(50 * time.Millisecond):
	}

	// First exceed (600+500=1100 > 1000): cancel fires, tripped=true.
	if tripped := r.accumulateRunTokenBudgetFromStream(context.Background(), run, "team-1", 500); !tripped {
		t.Fatal("first exceed must report tripped=true")
	}
	select {
	case got := <-cancelled:
		if got != "cancelled" {
			t.Fatalf("cancel func fired with %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("budget exceed must cancel the run via RunRegistry")
	}
	// Registered path: Cancel itself records the reason (P2.5 口径).
	if got := r.cancelReason("sess-1"); got != "team_token_budget_exceeded" {
		t.Errorf("cancelReason=%q, want team_token_budget_exceeded", got)
	}

	// Single-fire guard: further deltas accumulate but never re-fire.
	if tripped := r.accumulateRunTokenBudgetFromStream(context.Background(), run, "team-1", 500); tripped {
		t.Fatal("second exceed must report tripped=false (single-fire)")
	}
	if used := r.budgetUsed["run-1"]; used != 1_600 {
		t.Errorf("post-trip used=%d, want 1600 (accumulation continues)", used)
	}
	if !r.budgetTripped["run-1"] {
		t.Error("post-trip budgetTripped must stay true")
	}

	// Ungated run (never registered): hook is a no-op.
	if tripped := r.accumulateRunTokenBudgetFromStream(context.Background(), biz.TeamRunRecord{ID: "run-x", SessionID: "sess-x"}, "team-1", 1<<40); tripped {
		t.Fatal("ungated run must report tripped=false")
	}
}

// TestTripRunTokenBudget_ReasonFallbackWithoutRegistryEntry covers the
// RunTeamTest path: no StoreCancelable/activeRun entry exists, so
// RunRegistry.Cancel cannot record the cancel reason — tripRunTokenBudget
// must fall back to SetStatus so finishRunErr keeps the P2.5 口径.
func TestTripRunTokenBudget_ReasonFallbackWithoutRegistryEntry(t *testing.T) {
	r := &Runner{lg: loggateway.NewNoop()}
	reg := rt.NewRunRegistry()
	r.cfg.Runs = reg
	// NOTE: no StoreCancelable — mirrors RunTeamTest (bypasses orchestrator).

	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	r.registerRunTokenBudget("run-1", 1)
	if tripped := r.accumulateRunTokenBudgetFromStream(context.Background(), run, "team-1", 100); !tripped {
		t.Fatal("exceed must report tripped=true")
	}
	if got := r.cancelReason("sess-1"); got != "team_token_budget_exceeded" {
		t.Errorf("cancelReason=%q, want team_token_budget_exceeded (SetStatus fallback)", got)
	}
	entry, ok := reg.GetStatus("sess-1")
	if !ok || entry.Status != biz.SessionRunPhaseCancelled || entry.RunID != "run-1" {
		t.Errorf("status entry=%+v ok=%v, want cancelled run-1", entry, ok)
	}
}

// TestTripRunTokenBudget_BackfillsTokenIn pins t-dr-4：预算中止的 run 终态
// 走 finishRunErr（failed），finalizeTeamRun 的 success 路径 token 写回
// 不执行——跳闸时必须把观测值 used 回填 team_runs.token_in，否则终止 run
// 在审计/前端看似"零消耗"。已写入（>0）行不覆盖（success 边界竞态防护）。
func TestTripRunTokenBudget_BackfillsTokenIn(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning},
	}}
	r := &Runner{runReader: repo, runWriter: repo, lg: loggateway.NewNoop()}
	r.cfg.Runs = rt.NewRunRegistry()

	run := biz.TeamRunRecord{ID: "run-1", TeamID: "team-1", SessionID: "sess-1"}
	r.tripRunTokenBudget(context.Background(), run, "team-1", 1_100, 1_000)

	if got := repo.runs["run-1"].TokenIn; got != 1_100 {
		t.Fatalf("token_in = %d, want 1100（跳闸观测值回填）", got)
	}
	// 已写入不覆盖：第二次跳闸观测值更大，token_in 保持首次回填值。
	r.tripRunTokenBudget(context.Background(), run, "team-1", 9_999, 1_000)
	if got := repo.runs["run-1"].TokenIn; got != 1_100 {
		t.Fatalf("token_in = %d, want 1100（已写入不覆盖）", got)
	}
}
