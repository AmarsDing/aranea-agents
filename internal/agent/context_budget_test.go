package agent

import (
	"context"
	"testing"
)

// TestRecordContextBudget_NoCollectorNoop covers the nil-collector contract:
// callers never check for presence, recording into a bare context must be a
// silent no-op (no panic).
func TestRecordContextBudget_NoCollectorNoop(t *testing.T) {
	ctx := context.Background()
	RecordContextBudget(ctx, ContextBudgetCategoryStaticPrefix, 100)
	recordContextBudgetOnce(ctx, ContextBudgetCategoryStaticPrefix, 100)
	if got := ContextBudgetFromContext(ctx); got != nil {
		t.Fatalf("expected nil budget from bare ctx, got %v", got)
	}
}

// TestWithContextBudget_RoundTrip covers ctx mount + retrieval.
func TestWithContextBudget_RoundTrip(t *testing.T) {
	ctx, b := WithContextBudget(context.Background())
	if b == nil {
		t.Fatal("WithContextBudget returned nil budget")
	}
	if got := ContextBudgetFromContext(ctx); got != b {
		t.Fatalf("ContextBudgetFromContext = %p, want %p", got, b)
	}
}

// TestContextBudget_AddAccumulates covers repeated Add on one category.
func TestContextBudget_AddAccumulates(t *testing.T) {
	ctx, _ := WithContextBudget(context.Background())
	RecordContextBudget(ctx, ContextBudgetCategoryOtherDynamic, 10)
	RecordContextBudget(ctx, ContextBudgetCategoryOtherDynamic, 5)
	RecordContextBudget(ctx, ContextBudgetCategoryOtherDynamic, 1)
	snap := ContextBudgetFromContext(ctx).Snapshot()
	if got := snap.Chars[ContextBudgetCategoryOtherDynamic]; got != 16 {
		t.Fatalf("chars = %d, want 16", got)
	}
}

// TestContextBudget_MultiCategoryIndependent covers independent accumulation
// across categories.
func TestContextBudget_MultiCategoryIndependent(t *testing.T) {
	ctx, _ := WithContextBudget(context.Background())
	RecordContextBudget(ctx, ContextBudgetCategoryStaticPrefix, 100)
	RecordContextBudget(ctx, ContextBudgetCategoryMemoryL1, 7)
	RecordContextBudget(ctx, ContextBudgetCategoryKnowledgeCue, 3)
	RecordContextBudget(ctx, ContextBudgetCategoryStaticPrefix, 50)
	snap := ContextBudgetFromContext(ctx).Snapshot()
	if got := snap.Chars[ContextBudgetCategoryStaticPrefix]; got != 150 {
		t.Fatalf("static_prefix chars = %d, want 150", got)
	}
	if got := snap.Chars[ContextBudgetCategoryMemoryL1]; got != 7 {
		t.Fatalf("memory_l1 chars = %d, want 7", got)
	}
	if got := snap.Chars[ContextBudgetCategoryKnowledgeCue]; got != 3 {
		t.Fatalf("knowledge_cue chars = %d, want 3", got)
	}
	if _, ok := snap.Chars[ContextBudgetCategoryMemoryL4]; ok {
		t.Fatal("memory_l4 should be absent when never recorded")
	}
}

// TestContextBudget_SnapshotEstimation covers the chars/3.5 ceil estimate:
// zero, exact division, and non-divisible round-up.
func TestContextBudget_SnapshotEstimation(t *testing.T) {
	ctx, _ := WithContextBudget(context.Background())
	RecordContextBudget(ctx, ContextBudgetCategoryStaticPrefix, 0)
	RecordContextBudget(ctx, ContextBudgetCategoryMemoryL1, 7)     // 7/3.5 = 2 exactly
	RecordContextBudget(ctx, ContextBudgetCategoryMemoryL4, 8)     // 8/3.5 = 2.28 → 3
	RecordContextBudget(ctx, ContextBudgetCategoryKnowledgeCue, 1) // 1/3.5 → 1
	snap := ContextBudgetFromContext(ctx).Snapshot()
	cases := map[string]int{
		ContextBudgetCategoryStaticPrefix: 0,
		ContextBudgetCategoryMemoryL1:     2,
		ContextBudgetCategoryMemoryL4:     3,
		ContextBudgetCategoryKnowledgeCue: 1,
	}
	for cat, want := range cases {
		if got := snap.EstTokens[cat]; got != want {
			t.Fatalf("est_tokens[%s] = %d, want %d", cat, got, want)
		}
	}
	if got := snap.EstTotalInput; got != 6 {
		t.Fatalf("EstTotalInput = %d, want 6", got)
	}
}

// TestContextBudget_SetToolsCount covers tools count storage on the snapshot.
func TestContextBudget_SetToolsCount(t *testing.T) {
	ctx, b := WithContextBudget(context.Background())
	if got := b.Snapshot().ToolsCount; got != 0 {
		t.Fatalf("default ToolsCount = %d, want 0", got)
	}
	b.SetToolsCount(23)
	snap := ContextBudgetFromContext(ctx).Snapshot()
	if got := snap.ToolsCount; got != 23 {
		t.Fatalf("ToolsCount = %d, want 23", got)
	}
}

// TestRecordContextBudgetOnce_FirstWins covers the per-request dedupe used by
// BeforeModel hooks that re-fire on tool-loop model calls: only the first
// record per category counts.
func TestRecordContextBudgetOnce_FirstWins(t *testing.T) {
	ctx, b := WithContextBudget(context.Background())
	recordContextBudgetOnce(ctx, ContextBudgetCategoryStaticPrefix, 100)
	recordContextBudgetOnce(ctx, ContextBudgetCategoryStaticPrefix, 999)
	recordContextBudgetOnce(ctx, ContextBudgetCategoryMemoryL1, 50)
	snap := b.Snapshot()
	if got := snap.Chars[ContextBudgetCategoryStaticPrefix]; got != 100 {
		t.Fatalf("static_prefix chars = %d, want 100 (first write wins)", got)
	}
	if got := snap.Chars[ContextBudgetCategoryMemoryL1]; got != 50 {
		t.Fatalf("memory_l1 chars = %d, want 50", got)
	}
	if !b.has(ContextBudgetCategoryStaticPrefix) {
		t.Fatal("has(static_prefix) = false, want true")
	}
	if b.has(ContextBudgetCategorySkillGuidance) {
		t.Fatal("has(skill_guidance) = true, want false")
	}
}
