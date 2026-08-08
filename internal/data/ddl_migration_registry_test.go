package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// 20261130 memory_recall_defaults_fix (P0-3/P0-4): legacy rows with
// l4_enabled=true AND l0_inject_l4=false flip to true; l3_recall_min_score
// 0.55 drops to 0.35; explicit 0.00 (user disabled filtering) stays.
func TestMemoryRecallDefaultsFixMigration(t *testing.T) {
	client, rawDB := testhelper.SetupTestPG(t)
	ctx := context.Background()

	// Seed legacy rows with explicit old values (fresh Ent schema already
	// carries the NEW defaults, so old values must be set explicitly).
	client.AgentRuntimeSetting.Create().SetID("agent-l4-on").
		SetL4Enabled(true).SetL0InjectL4(false).SetL3RecallMinScore(0.55).SaveX(ctx)
	client.AgentRuntimeSetting.Create().SetID("agent-l4-off").
		SetL4Enabled(false).SetL0InjectL4(false).SetL3RecallMinScore(0.55).SaveX(ctx)
	client.AgentRuntimeSetting.Create().SetID("agent-filter-off").
		SetL4Enabled(true).SetL0InjectL4(false).SetL3RecallMinScore(0).SaveX(ctx)

	run := func() {
		t.Helper()
		if err := ddlMemoryRecallDefaultsFix(ctx, rawDB, client, DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("ddlMemoryRecallDefaultsFix: %v", err)
		}
	}
	run()

	l4On := client.AgentRuntimeSetting.GetX(ctx, "agent-l4-on")
	if !l4On.L0InjectL4 {
		t.Error("agent-l4-on: L0InjectL4 = false, want true (l4_enabled=true row flips)")
	}
	if l4On.L3RecallMinScore != 0.35 {
		t.Errorf("agent-l4-on: L3RecallMinScore = %v, want 0.35", l4On.L3RecallMinScore)
	}

	l4Off := client.AgentRuntimeSetting.GetX(ctx, "agent-l4-off")
	if l4Off.L0InjectL4 {
		t.Error("agent-l4-off: L0InjectL4 = true, want false (l4_enabled=false row keeps explicit off)")
	}
	if l4Off.L3RecallMinScore != 0.35 {
		t.Errorf("agent-l4-off: L3RecallMinScore = %v, want 0.35", l4Off.L3RecallMinScore)
	}

	filterOff := client.AgentRuntimeSetting.GetX(ctx, "agent-filter-off")
	if !filterOff.L0InjectL4 {
		t.Error("agent-filter-off: L0InjectL4 = false, want true")
	}
	if filterOff.L3RecallMinScore != 0 {
		t.Errorf("agent-filter-off: L3RecallMinScore = %v, want 0 (explicit filter-off untouched)", filterOff.L3RecallMinScore)
	}

	// Idempotent: second run must be a no-op (values unchanged).
	run()
	again := client.AgentRuntimeSetting.GetX(ctx, "agent-l4-on")
	if !again.L0InjectL4 || again.L3RecallMinScore != 0.35 {
		t.Errorf("re-run changed values: L0InjectL4=%v minScore=%v", again.L0InjectL4, again.L3RecallMinScore)
	}
}
