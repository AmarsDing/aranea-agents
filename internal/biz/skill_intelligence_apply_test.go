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

// ── ADR-3: agent create_skill auto-register branch ───────────────────────────

// agentApplyStoreStub serves a single unified row and records CAS writes.
// Only the methods exercised by applyAgentCreateSkill are implemented; the
// embedded nil interface panics on unexpected calls (fail-fast in tests).
type agentApplyStoreStub struct {
	UnifiedEvolutionStore
	row    *UnifiedEvolutionSuggestion
	casTo  string
	casHit bool
}

func (s *agentApplyStoreStub) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	if s.row != nil && s.row.ID == id {
		return s.row, nil
	}
	return nil, nil
}

func (s *agentApplyStoreStub) UpdateStatusCAS(_ context.Context, id string, from []string, to string, _, _ string) (bool, error) {
	if s.row == nil || s.row.ID != id {
		return false, nil
	}
	for _, f := range from {
		if s.row.Status == f {
			s.row.Status = to
			s.casTo = to
			s.casHit = true
			return true, nil
		}
	}
	return false, nil
}

// fakeRegistrar records RegisterSkill calls and stubs SkillExists.
type fakeRegistrar struct {
	exists     bool
	existsErr  error
	regErr     error
	regCalls   int
	gotAgentID string
	gotName    string
	gotBody    string
}

func (f *fakeRegistrar) SkillExists(_ context.Context, _, _ string) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeRegistrar) RegisterSkill(_ context.Context, agentID, name, skillMD string) error {
	f.regCalls++
	f.gotAgentID = agentID
	f.gotName = name
	f.gotBody = skillMD
	return f.regErr
}

func newAgentApprovedRow() *UnifiedEvolutionSuggestion {
	return &UnifiedEvolutionSuggestion{
		ID:              "sug-agent-1",
		TargetType:      EvolutionTargetAgent,
		TargetID:        "agent-1",
		ActionType:      EvolutionActionCreate,
		TriggerSource:   "pattern",
		Status:          string(UnifiedEvolutionStateApproved),
		DraftName:       "deploy-helper",
		DraftBody:       "# Deploy Helper\n\n## Guidance\nAutomate deploys.",
		LifecycleStatus: "draft",
		CreatedAt:       time.Now().UTC(),
	}
}

func newAgentApplyUsecase(store UnifiedEvolutionStore, registrar SkillRegistrationPort) *SkillIntelligenceUsecase {
	lg := loggateway.NewNoop()
	return NewSkillIntelligenceUsecase(nil, nil, store, nil, lg, SkillIntelligenceConfig{
		Registrar: registrar,
	})
}

func TestApplyApprovedSuggestion_AgentCreateSkill_Success(t *testing.T) {
	row := newAgentApprovedRow()
	store := &agentApplyStoreStub{row: row}
	reg := &fakeRegistrar{}
	uc := newAgentApplyUsecase(store, reg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), row.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if reg.regCalls != 1 {
		t.Fatalf("RegisterSkill calls = %d, want 1", reg.regCalls)
	}
	if reg.gotAgentID != "agent-1" || reg.gotName != "deploy-helper" {
		t.Errorf("RegisterSkill got (agent=%q, name=%q), want (agent-1, deploy-helper)", reg.gotAgentID, reg.gotName)
	}
	if !store.casHit || store.casTo != string(UnifiedEvolutionStateApplied) {
		t.Errorf("CAS = (hit=%v, to=%q), want (true, applied)", store.casHit, store.casTo)
	}
}

func TestApplyApprovedSuggestion_AgentCreateSkill_SkipsWhenNotApproved(t *testing.T) {
	row := newAgentApprovedRow()
	row.Status = string(UnifiedEvolutionStatePending)
	store := &agentApplyStoreStub{row: row}
	reg := &fakeRegistrar{}
	uc := newAgentApplyUsecase(store, reg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), row.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if reg.regCalls != 0 {
		t.Error("registrar must not be called for a pending suggestion")
	}
	if store.casHit {
		t.Error("CAS must not run for a pending suggestion")
	}
}

func TestApplyApprovedSuggestion_AgentCreateSkill_ExistingSkillIdempotent(t *testing.T) {
	row := newAgentApprovedRow()
	store := &agentApplyStoreStub{row: row}
	reg := &fakeRegistrar{exists: true}
	uc := newAgentApplyUsecase(store, reg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), row.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if reg.regCalls != 0 {
		t.Error("RegisterSkill must be skipped when the skill already exists")
	}
	if !store.casHit || store.casTo != string(UnifiedEvolutionStateApplied) {
		t.Errorf("CAS = (hit=%v, to=%q), want (true, applied) — existing skill still settles", store.casHit, store.casTo)
	}
}

func TestApplyApprovedSuggestion_AgentCreateSkill_RegisterErrorStaysApproved(t *testing.T) {
	row := newAgentApprovedRow()
	store := &agentApplyStoreStub{row: row}
	reg := &fakeRegistrar{regErr: errors.New("registry unavailable")}
	uc := newAgentApplyUsecase(store, reg)

	err := uc.ApplyApprovedSuggestion(context.Background(), row.ID)
	if err == nil {
		t.Fatal("expected register error to propagate")
	}
	if store.casHit {
		t.Error("CAS must not run when registration fails — suggestion stays approved")
	}
	if row.Status != string(UnifiedEvolutionStateApproved) {
		t.Errorf("status = %q, want approved (unchanged on register failure)", row.Status)
	}
}

func TestApplyApprovedSuggestion_AgentCreateSkill_NilRegistrarNoop(t *testing.T) {
	row := newAgentApprovedRow()
	store := &agentApplyStoreStub{row: row}
	uc := newAgentApplyUsecase(store, nil)

	if err := uc.ApplyApprovedSuggestion(context.Background(), row.ID); err != nil {
		t.Fatalf("expected nil error with nil registrar, got %v", err)
	}
	if store.casHit {
		t.Error("CAS must not run with nil registrar — suggestion stays approved")
	}
}
