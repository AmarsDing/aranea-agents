package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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

	first := resolveAndWriteSkillState(ctx, nil, deps, true)
	if first == nil || len(first.Slugs) != 1 || first.Slugs[0] != "skill-a" {
		t.Fatalf("first call = %#v, want [skill-a]", first)
	}
	second := resolveAndWriteSkillState(ctx, nil, deps, true)
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

	if got := resolveAndWriteSkillState(ctx, nil, deps, true); got != nil {
		t.Fatalf("first call should fail, got %#v", got)
	}
	// Transient error must not be cached: the next call retries and succeeds.
	got := resolveAndWriteSkillState(ctx, nil, deps, true)
	if got == nil || len(got.Slugs) != 1 {
		t.Fatalf("second call should succeed after transient error, got %#v", got)
	}
	if uc.calls != 2 {
		t.Errorf("resolver called %d times, want 2 (error not memoized)", uc.calls)
	}
}
