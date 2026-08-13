package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
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

// ── F5: skill_overview budget metering ──────────────────────────────────────

type fakeSkillBudgetRepo struct{ sums []trpcskill.Summary }

func (f *fakeSkillBudgetRepo) Summaries() []trpcskill.Summary { return f.sums }
func (f *fakeSkillBudgetRepo) Get(string) (*trpcskill.Skill, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeSkillBudgetRepo) Path(string) (string, error) { return "", nil }

func TestSkillOverviewBlockChars_Basic(t *testing.T) {
	repo := &fakeSkillBudgetRepo{sums: []trpcskill.Summary{
		{Name: "alpha", Description: "first skill"},
		{Name: "beta", Description: "second skill"},
	}}
	want := "Available skills:\n- alpha: first skill\n- beta: second skill\n"
	if got := skillOverviewBlockChars(context.Background(), repo, nil); got != utf8.RuneCountInString(want) {
		t.Fatalf("chars = %d, want %d (%q)", got, utf8.RuneCountInString(want), want)
	}
}

func TestSkillOverviewBlockChars_FilterApplied(t *testing.T) {
	repo := &fakeSkillBudgetRepo{sums: []trpcskill.Summary{
		{Name: "alpha", Description: "first skill"},
		{Name: "beta", Description: "second skill"},
	}}
	filter := func(_ context.Context, s trpcskill.Summary) bool {
		return !strings.EqualFold(s.Name, "beta")
	}
	want := "Available skills:\n- alpha: first skill\n"
	if got := skillOverviewBlockChars(context.Background(), repo, filter); got != utf8.RuneCountInString(want) {
		t.Fatalf("chars = %d, want %d (filter must drop beta)", got, utf8.RuneCountInString(want))
	}
}

func TestSkillOverviewBlockChars_Empty(t *testing.T) {
	if got := skillOverviewBlockChars(context.Background(), nil, nil); got != 0 {
		t.Fatalf("nil repo chars = %d, want 0", got)
	}
	if got := skillOverviewBlockChars(context.Background(), &fakeSkillBudgetRepo{}, nil); got != 0 {
		t.Fatalf("empty repo chars = %d, want 0", got)
	}
}

// TestSkillOverviewBudgetHook_RecordsOnce invokes the hook twice (tool-loop
// re-entry) and asserts the first measurement wins; a bare ctx (no collector)
// must be a silent no-op.
func TestSkillOverviewBudgetHook_RecordsOnce(t *testing.T) {
	repo := &fakeSkillBudgetRepo{sums: []trpcskill.Summary{
		{Name: "alpha", Description: "first skill"},
	}}
	hook := newContextBudgetSkillOverviewBeforeHook(repo, nil)
	if hook == nil {
		t.Fatal("hook = nil, want non-nil for non-nil repo")
	}
	bm, ok := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if !ok {
		t.Fatal("hook does not implement HandleBeforeModel")
	}

	// No collector: silent no-op.
	if _, err := bm.HandleBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("bare ctx call err = %v", err)
	}

	ctx, b := WithContextBudget(context.Background())
	args := &trpcmodel.BeforeModelArgs{}
	if _, err := bm.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("first call err = %v", err)
	}
	if _, err := bm.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("second call err = %v", err)
	}
	want := "Available skills:\n- alpha: first skill\n"
	snap := b.Snapshot()
	if got := snap.Chars[ContextBudgetCategorySkillOverview]; got != utf8.RuneCountInString(want) {
		t.Fatalf("skill_overview chars = %d, want %d (recorded once)", got, utf8.RuneCountInString(want))
	}
	if _, ok := snap.EstTokens[ContextBudgetCategorySkillOverview]; !ok {
		t.Fatal("skill_overview missing from EstTokens snapshot")
	}
}

func TestSkillOverviewBudgetHook_NilRepo(t *testing.T) {
	if hook := newContextBudgetSkillOverviewBeforeHook(nil, nil); hook != nil {
		t.Fatal("hook = non-nil for nil repo, want nil (no registration)")
	}
}

// ── N3: history budget metering ─────────────────────────────────────────────

// TestContextBudgetHistoryHook_CountsNonSystemOnly covers the core metering
// rule: only non-system messages (history + current user input) count toward
// the history category; system messages (static prefix + injected cues) are
// excluded. First model call wins on tool-loop re-entry.
func TestContextBudgetHistoryHook_CountsNonSystemOnly(t *testing.T) {
	hook := newContextBudgetHistoryBeforeHook()
	if hook == nil {
		t.Fatal("hook = nil, want non-nil")
	}

	// Bare ctx: silent no-op.
	if _, err := hook.HandleBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("bare ctx call err = %v", err)
	}

	ctx, b := WithContextBudget(context.Background())
	args := &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{
			Messages: []trpcmodel.Message{
				trpcmodel.NewSystemMessage("static prefix and injected cues"),
				trpcmodel.NewUserMessage("first user turn"),
				trpcmodel.NewAssistantMessage("assistant reply"),
				trpcmodel.NewUserMessage("current question"),
			},
		},
	}
	if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("first call err = %v", err)
	}
	want := utf8.RuneCountInString("first user turn") +
		utf8.RuneCountInString("assistant reply") +
		utf8.RuneCountInString("current question")
	snap := b.Snapshot()
	if got := snap.Chars[ContextBudgetCategoryHistory]; got != want {
		t.Fatalf("history chars = %d, want %d (non-system only)", got, want)
	}

	// Tool-loop re-entry with grown messages: first measurement wins.
	args.Request.Messages = append(args.Request.Messages,
		trpcmodel.NewUserMessage("tool result echo that must not be re-counted"))
	if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("second call err = %v", err)
	}
	if got := b.Snapshot().Chars[ContextBudgetCategoryHistory]; got != want {
		t.Fatalf("history chars after re-entry = %d, want %d (recorded once)", got, want)
	}
}

// TestContextBudgetHistoryHook_NilRequest covers the defensive nil paths:
// no panic, nothing recorded.
func TestContextBudgetHistoryHook_NilRequest(t *testing.T) {
	hook := newContextBudgetHistoryBeforeHook()
	ctx, b := WithContextBudget(context.Background())
	if _, err := hook.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("nil request err = %v", err)
	}
	if b.has(ContextBudgetCategoryHistory) {
		t.Fatal("history recorded for nil request, want skipped")
	}
}
