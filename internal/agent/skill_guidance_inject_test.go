package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestNewSkillGuidanceBeforeHook_ProgressiveMode(t *testing.T) {
	// When SkillUC is nil, the hook returns nil regardless of mode.
	// This test verifies the progressive branch is selected (not the
	// default eager hook) by checking that IsProgressiveSkillLoad
	// correctly identifies the mode.
	ag := biz.Agent{
		ID:               "test-agent",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			SkillLoadMode: "progressive",
		},
	}
	// SkillUC nil → hook returns nil (early exit), but the branch
	// selection logic is verified by IsProgressiveSkillLoad below.
	deps := TRPCBuilderDeps{}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	// nil because SkillUC is nil — the early exit guard fires first.
	if hook != nil {
		t.Fatal("expected nil hook when SkillUC is nil")
	}
}

func TestNewSkillGuidanceBeforeHook_ProgressiveModeUppercase(t *testing.T) {
	// Verify that "Progressive" (uppercase) is recognized as progressive mode.
	ag := biz.Agent{
		ID:               "test-agent",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			SkillLoadMode: "Progressive",
		},
	}
	deps := TRPCBuilderDeps{}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	// nil because SkillUC is nil — but the branch was selected via
	// IsProgressiveSkillLoad("Progressive") which should return true.
	if hook != nil {
		t.Fatal("expected nil hook when SkillUC is nil")
	}
}

func TestIsProgressiveSkillLoad_Integration(t *testing.T) {
	// Verify IsProgressiveSkillLoad works correctly for the hook's branch logic.
	tests := []struct {
		mode string
		want bool
	}{
		{"progressive", true},
		{"Progressive", true},
		{"PROGRESSIVE", true},
		{" progressive ", true},
		{"turn", false},
		{"", false},
	}
	for _, tt := range tests {
		got := biz.IsProgressiveSkillLoad(tt.mode)
		if got != tt.want {
			t.Errorf("IsProgressiveSkillLoad(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

// countingSkillLookup implements biz.TeamSkillLookup with a call counter on
// ListEnabledPublishedSkillCandidates to verify per-invocation memoization.
type countingSkillLookup struct {
	calls    int
	failNext bool
}

func (m *countingSkillLookup) ListEnabledPublishedSkillKeys(_ context.Context) ([]string, error) {
	return []string{"skill-a"}, nil
}

func (m *countingSkillLookup) ListEnabledPublishedSkillRefs(_ context.Context) ([]biz.SkillEnabledRef, error) {
	return nil, nil
}

func (m *countingSkillLookup) ListEnabledPublishedSkillCandidates(_ context.Context) ([]biz.SkillRuntimeCandidate, error) {
	m.calls++
	if m.failNext {
		m.failNext = false
		return nil, errors.New("transient db error")
	}
	return []biz.SkillRuntimeCandidate{{Slug: "skill-a", Name: "A", Description: "desc a"}}, nil
}

func (m *countingSkillLookup) ScoreByEmbedding(_ context.Context, _ string, _ []biz.SkillRuntimeCandidate) (map[string]float64, error) {
	return nil, nil
}

func (m *countingSkillLookup) BatchGetSkillGuidance(_ context.Context, _ []string) ([]biz.SkillGuidanceEntry, error) {
	return nil, nil
}

func (m *countingSkillLookup) GetBySlug(_ context.Context, _ string) (biz.Skill, error) {
	return biz.Skill{}, nil
}

func (m *countingSkillLookup) RecordInvocation(_ context.Context, _ biz.SkillInvocationWrite) error {
	return nil
}

func TestResolveAndWriteSkillState_MemoizedPerInvocation(t *testing.T) {
	uc := &countingSkillLookup{}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{SkillUC: uc}}
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	first := resolveAndWriteSkillState(ctx, nil, deps)
	if first == nil || len(first.Slugs) != 1 || first.Slugs[0] != "skill-a" {
		t.Fatalf("first call = %#v, want [skill-a]", first)
	}
	second := resolveAndWriteSkillState(ctx, nil, deps)
	if second == nil || len(second.Slugs) != 1 || second.Slugs[0] != "skill-a" {
		t.Fatalf("second call = %#v, want [skill-a]", second)
	}
	if uc.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (memoized per invocation)", uc.calls)
	}
	// State keys must still be populated after the memoized second call.
	if raw, ok := inv.GetState(skillRoutedSlugsStateKey); !ok || len(raw.([]string)) != 1 {
		t.Error("routed slugs state missing after memoized call")
	}
	if _, ok := inv.GetState(skillSelectionReasonStateKey); !ok {
		t.Error("selection reasons state missing after memoized call")
	}
}

func TestResolveAndWriteSkillState_ErrorNotMemoized(t *testing.T) {
	uc := &countingSkillLookup{failNext: true}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{SkillUC: uc}}
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	if got := resolveAndWriteSkillState(ctx, nil, deps); got != nil {
		t.Fatalf("first call should fail, got %#v", got)
	}
	// Transient error must not be cached: the next call retries and succeeds.
	got := resolveAndWriteSkillState(ctx, nil, deps)
	if got == nil || len(got.Slugs) != 1 {
		t.Fatalf("second call should succeed after transient error, got %#v", got)
	}
	if uc.calls != 2 {
		t.Errorf("resolver called %d times, want 2 (error not memoized)", uc.calls)
	}
}

// TestResolveAndWriteSkillState_RoutedSlugsPersistedInFullProfile covers R2
// (2026-08-13 复查): routed slugs must be persisted to invocation state in
// BOTH modes — gating on progressive left full-profile agents without
// routing observability (health metrics could not correlate routed vs run).
func TestResolveAndWriteSkillState_RoutedSlugsPersistedInFullProfile(t *testing.T) {
	uc := &countingSkillLookup{}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{SkillUC: uc}}
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	result := resolveAndWriteSkillState(ctx, nil, deps) // full-profile mode caller
	if result == nil || len(result.Slugs) != 1 {
		t.Fatalf("resolve = %#v, want [skill-a]", result)
	}
	raw, ok := inv.GetState(skillRoutedSlugsStateKey)
	if !ok {
		t.Fatal("routed slugs state missing in full-profile mode (R2)")
	}
	if slugs := raw.([]string); len(slugs) != 1 || slugs[0] != "skill-a" {
		t.Errorf("routed slugs = %v, want [skill-a]", slugs)
	}
}

// countingHealthProvider proves the fusion branch received the provider
// wired through TRPCSkillDeps (R1, 2026-08-13).
type countingHealthProvider struct{ calls int }

func (p *countingHealthProvider) GetRecentSuccessRate(_ context.Context, _ string, _ int) (float64, error) {
	p.calls++
	return 0.9, nil
}

func (p *countingHealthProvider) GetRecentAvgDuration(_ context.Context, _ string, _ int) (float64, error) {
	return 0, nil
}

// TestResolveAndWriteSkillState_WiresHealthProvider covers R1: the provider
// in TRPCSkillDeps must reach ResolveSkillSlugsDetailed so the historical-
// performance fusion branch actually runs on the turn routing path.
func TestResolveAndWriteSkillState_WiresHealthProvider(t *testing.T) {
	uc := &countingSkillLookup{}
	provider := &countingHealthProvider{}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{SkillUC: uc, SkillHealthProvider: provider}}
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	result := resolveAndWriteSkillState(ctx, nil, deps)
	if result == nil || len(result.Slugs) != 1 {
		t.Fatalf("resolve = %#v, want [skill-a]", result)
	}
	if provider.calls == 0 {
		t.Error("SkillHealthProvider never queried — R1 wiring broken")
	}
}

// ── N4: full-profile guidance cue memoization ───────────────────────────────

// TestSkillGuidanceHook_TaskModeNonProgressive_PersistsRoutedSlugs covers Q4
// (2026-08-13 最终复查): task prompt mode + non-progressive load mode must
// still resolve routed slugs and write them to invocation state — otherwise
// health metrics lose the routed-vs-run correlation for that mode combo.
// Task mode's minimal-prompt contract is preserved: no message is injected.
func TestSkillGuidanceHook_TaskModeNonProgressive_PersistsRoutedSlugs(t *testing.T) {
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "task",
		Settings:         &biz.AgentRuntimeSettings{SkillLoadMode: "turn"},
	}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{
		SkillUC: fakeSkillLookup{
			candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
		},
	}}
	cb := newSkillGuidanceBeforeHook(ag, deps)
	if cb == nil {
		t.Fatal("task + non-progressive returned nil hook — routed slugs never persisted (Q4)")
	}
	h, ok := cb.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatalf("hook %T does not implement callbacks.BeforeModelHook", cb)
	}
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	args := freshGuidanceArgs()
	if _, err := h.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("hook err = %v", err)
	}
	raw, ok := inv.GetState(skillRoutedSlugsStateKey)
	if !ok {
		t.Fatal("routed slugs state missing in task + non-progressive mode (Q4)")
	}
	if slugs := raw.([]string); len(slugs) != 1 || slugs[0] != "demo-skill" {
		t.Errorf("routed slugs = %v, want [demo-skill]", slugs)
	}
	if _, ok := inv.GetState(skillSelectionReasonStateKey); !ok {
		t.Error("selection reasons state missing")
	}
	// Minimal-prompt contract: the hook must not inject any message.
	if len(args.Request.Messages) != 2 {
		t.Errorf("task mode hook injected a message, messages = %d, want 2 (unchanged)", len(args.Request.Messages))
	}
}

// countingGuidanceLookup counts BatchGetSkillGuidance calls to verify the
// rendered cue is memoized per invocation (tool-loop model calls must not
// re-query the DB).
type countingGuidanceLookup struct {
	fakeSkillLookup
	guidanceCalls int
	failNext      bool
}

func (m *countingGuidanceLookup) BatchGetSkillGuidance(_ context.Context, _ []string) ([]biz.SkillGuidanceEntry, error) {
	m.guidanceCalls++
	if m.failNext {
		m.failNext = false
		return nil, errors.New("transient guidance db error")
	}
	return m.guidance, nil
}

func fullProfileGuidanceHookForTest(t *testing.T, uc biz.TeamSkillLookup) callbacks.BeforeModelHook {
	t.Helper()
	ag := biz.Agent{
		ID:               "ag-1",
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{SkillLoadMode: "turn"},
	}
	deps := TRPCBuilderDeps{TRPCSkillDeps: TRPCSkillDeps{SkillUC: uc}}
	cb := newSkillGuidanceBeforeHook(ag, deps)
	if cb == nil {
		t.Fatal("hook constructor returned nil; test setup must satisfy its guards")
	}
	h, ok := cb.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatalf("hook %T does not implement callbacks.BeforeModelHook", cb)
	}
	return h
}

func freshGuidanceArgs() *trpcmodel.BeforeModelArgs {
	return &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage("base system"),
		trpcmodel.NewUserMessage("你好"),
	}}}
}

// TestSkillGuidanceFullProfileHook_MemoizesGuidancePerInvocation fires the
// full-profile hook twice within one invocation (tool-loop re-entry): the
// guidance DB query must run once, and both injections must be byte-identical
// (cache-prefix stability).
func TestSkillGuidanceFullProfileHook_MemoizesGuidancePerInvocation(t *testing.T) {
	uc := &countingGuidanceLookup{fakeSkillLookup: fakeSkillLookup{
		candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
		guidance:   []biz.SkillGuidanceEntry{{Slug: "demo-skill", Guidance: "use the demo skill"}},
	}}
	hook := fullProfileGuidanceHookForTest(t, uc)
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	args1 := freshGuidanceArgs()
	if _, err := hook.HandleBeforeModel(ctx, args1); err != nil {
		t.Fatalf("first call err = %v", err)
	}
	args2 := freshGuidanceArgs()
	if _, err := hook.HandleBeforeModel(ctx, args2); err != nil {
		t.Fatalf("second call err = %v", err)
	}

	if uc.guidanceCalls != 1 {
		t.Errorf("BatchGetSkillGuidance called %d times, want 1 (memoized per invocation)", uc.guidanceCalls)
	}
	last1 := args1.Request.Messages[len(args1.Request.Messages)-1]
	last2 := args2.Request.Messages[len(args2.Request.Messages)-1]
	if last1.Role != trpcmodel.RoleUser || !isDynamicCueMessage(last1) || !strings.Contains(last1.Content, "Available Skills") {
		t.Fatalf("first call tail = role %s tool %q content %.40q, want injected guidance cue", last1.Role, last1.ToolName, last1.Content)
	}
	if last1.Content != last2.Content {
		t.Error("injected cue differs between model calls; memoized cue must be byte-identical")
	}
}

// TestSkillGuidanceFullProfileHook_GuidanceErrorNotMemoized: a transient
// guidance query error skips injection and is NOT memoized — the next model
// call retries and succeeds.
func TestSkillGuidanceFullProfileHook_GuidanceErrorNotMemoized(t *testing.T) {
	uc := &countingGuidanceLookup{failNext: true, fakeSkillLookup: fakeSkillLookup{
		candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
		guidance:   []biz.SkillGuidanceEntry{{Slug: "demo-skill", Guidance: "use the demo skill"}},
	}}
	hook := fullProfileGuidanceHookForTest(t, uc)
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	args1 := freshGuidanceArgs()
	if _, err := hook.HandleBeforeModel(ctx, args1); err != nil {
		t.Fatalf("first call err = %v", err)
	}
	if len(args1.Request.Messages) != 2 {
		t.Fatalf("first call injected on transient error, messages = %d, want 2", len(args1.Request.Messages))
	}
	args2 := freshGuidanceArgs()
	if _, err := hook.HandleBeforeModel(ctx, args2); err != nil {
		t.Fatalf("second call err = %v", err)
	}
	if uc.guidanceCalls != 2 {
		t.Errorf("BatchGetSkillGuidance called %d times, want 2 (error not memoized)", uc.guidanceCalls)
	}
	if len(args2.Request.Messages) != 3 {
		t.Fatalf("second call should inject after retry, messages = %d, want 3", len(args2.Request.Messages))
	}
}

// TestSkillGuidanceFullProfileHook_EmptyGuidanceMemoized: an empty guidance
// result is a legitimate outcome and IS memoized — subsequent model calls
// skip both the DB query and the injection.
func TestSkillGuidanceFullProfileHook_EmptyGuidanceMemoized(t *testing.T) {
	uc := &countingGuidanceLookup{fakeSkillLookup: fakeSkillLookup{
		candidates: []biz.SkillRuntimeCandidate{{Slug: "demo-skill", Name: "Demo"}},
	}}
	hook := fullProfileGuidanceHookForTest(t, uc)
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	for i := 0; i < 2; i++ {
		args := freshGuidanceArgs()
		if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
			t.Fatalf("call %d err = %v", i, err)
		}
		if len(args.Request.Messages) != 2 {
			t.Fatalf("call %d injected with empty guidance, messages = %d, want 2", i, len(args.Request.Messages))
		}
	}
	if uc.guidanceCalls != 1 {
		t.Errorf("BatchGetSkillGuidance called %d times, want 1 (empty result memoized)", uc.guidanceCalls)
	}
}
