package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// captureReloader records Reload invocations for assertion.
type captureReloader struct {
	called  bool
	skillID string
	draft   string
	parent  string
	reason  string
	err     error
}

func (m *captureReloader) Reload(_ context.Context, skillID, draftBody, parentVersionID, evolutionReason string) error {
	m.called = true
	m.skillID = skillID
	m.draft = draftBody
	m.parent = parentVersionID
	m.reason = evolutionReason
	return m.err
}

// newApplyTestUsecase builds a SkillIntelligenceUsecase with the given
// reloader (nil allowed) over a bridge store seeded with suggestions.
func newApplyTestUsecase(bridge *mockEvolutionStoreBridge, reloader SkillReloader) *SkillIntelligenceUsecase {
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(nil, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	return NewSkillIntelligenceUsecase(scorer, reporter, bridge, nil, lg, SkillIntelligenceConfig{
		Reloader: reloader,
	})
}

func seedApprovedReadySuggestion() SkillEvolutionSuggestion {
	return SkillEvolutionSuggestion{
		ID:              "sug-apply-1",
		SkillID:         "skill-1",
		Type:            EvoSuggestionFixFailure,
		Status:          EvoSuggestionApproved,
		TriggerReason:   "7d success rate 45% below threshold 60%",
		DraftSkillBody:  "# Improved Skill\n\n## Guidance\nDo better.",
		LifecycleStatus: EvoLifecycleReady,
		SandboxPassed:   true,
		CreatedAt:       time.Now().UTC(),
	}
}

func TestApplyApprovedSuggestion_NilReloaderNoop(t *testing.T) {
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seedApprovedReadySuggestion()}}
	uc := newApplyTestUsecase(bridge, nil)
	if err := uc.ApplyApprovedSuggestion(context.Background(), "sug-apply-1"); err != nil {
		t.Fatalf("expected nil error with nil reloader, got %v", err)
	}
	// Suggestion must remain untouched (still approved + ready).
	got := bridge.suggestions[0]
	if got.Status != EvoSuggestionApproved || got.LifecycleStatus != EvoLifecycleReady {
		t.Errorf("suggestion mutated with nil reloader: status=%s lifecycle=%s", got.Status, got.LifecycleStatus)
	}
}

func TestApplyApprovedSuggestion_SkipsWhenNotApproved(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.Status = EvoSuggestionPending
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rel.called {
		t.Error("reloader must not be called for a pending suggestion")
	}
}

func TestApplyApprovedSuggestion_SkipsWhenLifecycleNotReady(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.LifecycleStatus = EvoLifecycleDraft
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rel.called {
		t.Error("reloader must not be called when lifecycle is draft")
	}
}

func TestApplyApprovedSuggestion_SkipsWhenSandboxNotPassed(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.SandboxPassed = false
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rel.called {
		t.Error("reloader must not be called when sandbox_passed is false")
	}
}

func TestApplyApprovedSuggestion_SkipsWhenDraftEmpty(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.DraftSkillBody = "   "
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rel.called {
		t.Error("reloader must not be called with an empty draft body")
	}
}

func TestApplyApprovedSuggestion_Success(t *testing.T) {
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seedApprovedReadySuggestion()}}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), "sug-apply-1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !rel.called {
		t.Fatal("reloader must be called for an approved+ready+passed suggestion")
	}
	if rel.skillID != "skill-1" {
		t.Errorf("skillID = %q, want %q", rel.skillID, "skill-1")
	}
	if rel.draft != "# Improved Skill\n\n## Guidance\nDo better." {
		t.Errorf("draft = %q, want seeded draft body", rel.draft)
	}
	if !strings.Contains(rel.reason, "fix_failure") || !strings.Contains(rel.reason, "7d success rate") {
		t.Errorf("reason = %q, want fallback containing type and trigger reason", rel.reason)
	}

	got := bridge.suggestions[0]
	if got.LifecycleStatus != EvoLifecycleApplied {
		t.Errorf("lifecycle = %q, want %q", got.LifecycleStatus, EvoLifecycleApplied)
	}
	if got.Status != EvoSuggestionApplied {
		t.Errorf("status = %q, want %q", got.Status, EvoSuggestionApplied)
	}
}

func TestApplyApprovedSuggestion_ReloadErrorStaysReady(t *testing.T) {
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seedApprovedReadySuggestion()}}
	rel := &captureReloader{err: errors.New("version registration failed")}
	uc := newApplyTestUsecase(bridge, rel)

	err := uc.ApplyApprovedSuggestion(context.Background(), "sug-apply-1")
	if err == nil {
		t.Fatal("expected reload error to propagate")
	}
	got := bridge.suggestions[0]
	if got.LifecycleStatus != EvoLifecycleReady {
		t.Errorf("lifecycle = %q, want %q (unchanged on reload failure)", got.LifecycleStatus, EvoLifecycleReady)
	}
	if got.Status != EvoSuggestionApproved {
		t.Errorf("status = %q, want %q (unchanged on reload failure)", got.Status, EvoSuggestionApproved)
	}
}

func TestApplyApprovedSuggestion_NotFound(t *testing.T) {
	bridge := &mockEvolutionStoreBridge{}
	rel := &captureReloader{}
	uc := newApplyTestUsecase(bridge, rel)

	if err := uc.ApplyApprovedSuggestion(context.Background(), "missing"); err == nil {
		t.Fatal("expected NotFound error for missing suggestion")
	}
	if rel.called {
		t.Error("reloader must not be called for a missing suggestion")
	}
}
