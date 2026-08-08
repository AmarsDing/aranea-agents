package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── in-memory fakes ──────────────────────────────────────────────────────────

type siRunStore struct {
	mu          sync.Mutex
	run         *SelfImprovementRun
	others      []SelfImprovementRun          // List 额外返回的行（按 filter.Status 过滤）
	transitions [][2]SelfImprovementRunStatus // (from → to) per Update
	attempts    int
}

func (s *siRunStore) GetByID(_ context.Context, id string) (*SelfImprovementRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil && s.run.ID == id {
		cp := *s.run
		return &cp, nil
	}
	for i := range s.others {
		if s.others[i].ID == id {
			cp := s.others[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (s *siRunStore) GetBySuggestionID(_ context.Context, suggestionID string) (*SelfImprovementRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil || s.run.SuggestionID != suggestionID {
		return nil, nil
	}
	cp := *s.run
	return &cp, nil
}
func (s *siRunStore) List(_ context.Context, f RunFilter) ([]SelfImprovementRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []SelfImprovementRun
	if s.run != nil && (f.Status == "" || s.run.Status == f.Status) {
		out = append(out, *s.run)
	}
	for _, r := range s.others {
		if f.Status == "" || r.Status == f.Status {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *siRunStore) ListObservingDue(_ context.Context, _ time.Time) ([]SelfImprovementRun, error) {
	return nil, nil
}
func (s *siRunStore) Count(_ context.Context, f RunFilter) (int, error) {
	runs, err := s.List(context.Background(), f)
	if err != nil {
		return 0, err
	}
	return len(runs), nil
}
func (s *siRunStore) ListTerminalPendingOutcome(_ context.Context, _ int) ([]SelfImprovementRun, error) {
	return nil, nil
}
func (s *siRunStore) Create(_ context.Context, run *SelfImprovementRun) error {
	s.run = run
	return nil
}
func (s *siRunStore) Update(_ context.Context, run *SelfImprovementRun, from SelfImprovementRunStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil && s.run.ID == run.ID {
		if s.run.Status != from {
			return fmt.Errorf("CAS conflict: stored=%v from=%v", s.run.Status, from)
		}
		if from != run.Status { // 只记录真实状态迁移（同状态 Update 仅持久化字段）
			s.transitions = append(s.transitions, [2]SelfImprovementRunStatus{from, run.Status})
		}
		cp := *run
		s.run = &cp
		return nil
	}
	for i := range s.others {
		if s.others[i].ID == run.ID {
			if s.others[i].Status != from {
				return fmt.Errorf("CAS conflict: stored=%v from=%v", s.others[i].Status, from)
			}
			if from != run.Status {
				s.transitions = append(s.transitions, [2]SelfImprovementRunStatus{from, run.Status})
			}
			s.others[i] = *run
			return nil
		}
	}
	return fmt.Errorf("run %s not found", run.ID)
}
func (s *siRunStore) RecordAttempt(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	if s.run != nil {
		s.run.Attempts++
	}
	return nil
}

type siSugReader struct{ sug *UnifiedEvolutionSuggestion }

func (r *siSugReader) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	if r.sug == nil || r.sug.ID != id {
		return nil, nil
	}
	return r.sug, nil
}
func (r *siSugReader) ListByTarget(_ context.Context, _, _, _ string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *siSugReader) CountByTarget(_ context.Context, _, _, _ string) (int, error) { return 0, nil }
func (r *siSugReader) ListByTargetAndAction(_ context.Context, _, _, _, _ string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *siSugReader) CountByTargetAndAction(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, nil
}

type siFakeSandbox struct {
	worktreePath string
	cleanupCalls int
	appliedDiffs []string
	gateCalls    []SandboxGateKind
	gateFn       func(gate SandboxGateKind, gateCallIdx int) SandboxGateResult
	prepareErr   error
}

func (s *siFakeSandbox) PrepareWorktree(_ context.Context, runID, _ string) (string, func(), error) {
	if s.prepareErr != nil {
		return "", nil, s.prepareErr
	}
	p := s.worktreePath
	if p == "" {
		p = "/tmp/wt-" + runID
	}
	return p, func() { s.cleanupCalls++ }, nil
}
func (s *siFakeSandbox) ApplyDiff(_ context.Context, _, diff string) error {
	s.appliedDiffs = append(s.appliedDiffs, diff)
	return nil
}
func (s *siFakeSandbox) RunGate(_ context.Context, _ string, gate SandboxGateKind, _ []string) (SandboxGateResult, error) {
	s.gateCalls = append(s.gateCalls, gate)
	if s.gateFn != nil {
		return s.gateFn(gate, len(s.gateCalls)-1), nil
	}
	return SandboxGateResult{Gate: gate, Passed: true}, nil
}

// ── stage func adapters ──────────────────────────────────────────────────────

type siAnalystFn func(ctx context.Context, run *SelfImprovementRun, sug *UnifiedEvolutionSuggestion) (*Diagnosis, error)

func (f siAnalystFn) Analyze(ctx context.Context, run *SelfImprovementRun, sug *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
	return f(ctx, run, sug)
}

type siPatcherFn func(ctx context.Context, req SIPatchRequest) (*PatcherOutput, error)

func (f siPatcherFn) Patch(ctx context.Context, req SIPatchRequest) (*PatcherOutput, error) {
	return f(ctx, req)
}

type siCriticFn func(ctx context.Context, run *SelfImprovementRun, patch *PatcherOutput) (*CriticReport, error)

func (f siCriticFn) Review(ctx context.Context, run *SelfImprovementRun, patch *PatcherOutput) (*CriticReport, error) {
	return f(ctx, run, patch)
}

// ── test fixtures ────────────────────────────────────────────────────────────

// siPipelineFixture builds the usecase with default happy-path mocks; mutate
// may adjust deps before construction.
func siPipelineFixture(maxAttempts int, mutate func(*SelfImprovementPipelineDeps)) (*SelfImprovementPipelineUsecase, *siRunStore, *siFakeSandbox, *[]SIPatchRequest) {
	store := &siRunStore{run: &SelfImprovementRun{
		ID: "run-1", SuggestionID: "sug-1", Status: RunStatusDetected, TriggerSource: TriggerSourceErrorCluster,
	}}
	sug := &siSugReader{sug: &UnifiedEvolutionSuggestion{
		ID: "sug-1", TargetType: EvolutionTargetPlatform, TriggerSource: TriggerSourceErrorCluster,
		DraftName: "fix nil deref", DraftBody: "x.go nil deref on empty input",
	}}
	sandbox := &siFakeSandbox{}
	patchReqs := &[]SIPatchRequest{}
	deps := SelfImprovementPipelineDeps{
		Analyst: siAnalystFn(func(context.Context, *SelfImprovementRun, *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			return &Diagnosis{RootCause: "nil deref", AffectedFiles: []string{"internal/biz/x.go"}, ImpactScope: "local", FixStrategy: "guard", Confidence: 0.9}, nil
		}),
		Patcher: siPatcherFn(func(_ context.Context, req SIPatchRequest) (*PatcherOutput, error) {
			*patchReqs = append(*patchReqs, req)
			return &PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 5), Kind: PatchKindCode, Summary: "guard nil"}, nil
		}),
		Critic: siCriticFn(func(context.Context, *SelfImprovementRun, *PatcherOutput) (*CriticReport, error) {
			return &CriticReport{IsSafe: true, RiskLevel: "low"}, nil
		}),
		Sandbox:      sandbox,
		Suggestions:  sug,
		RunReader:    store,
		RunWriter:    store,
		Classifier:   NewSIRiskClassifier(),
		MaxAttempts:  maxAttempts,
		MaxDiffLines: 500,
		Lg:           loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	return NewSelfImprovementPipelineUsecase(deps), store, sandbox, patchReqs
}

func transitionSeq(store *siRunStore) string {
	var b strings.Builder
	for _, tr := range store.transitions {
		fmt.Fprintf(&b, "%s>%s ", tr[0], tr[1])
	}
	return strings.TrimSpace(b.String())
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSIPipeline_HappyPath(t *testing.T) {
	uc, store, sandbox, patchReqs := siPipelineFixture(3, nil)
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("final status = %s, want awaiting_governance", store.run.Status)
	}
	want := "detected>diagnosing diagnosing>patching patching>verifying verifying>awaiting_governance"
	if got := transitionSeq(store); got != want {
		t.Errorf("transitions = %q, want %q", got, want)
	}
	if store.run.Diagnosis == nil || store.run.Diagnosis.RootCause != "nil deref" {
		t.Error("diagnosis not persisted")
	}
	if store.run.Diff == "" || store.run.PatchKind != PatchKindCode {
		t.Error("patch not persisted")
	}
	if len(store.run.VerificationReport) != 4 {
		t.Errorf("verification report gates = %d, want 4（G1-G3 + G5 skipped 记录）", len(store.run.VerificationReport))
	} else if last := store.run.VerificationReport[3]; last.Gate != SandboxGateEvalBase || !last.Passed || !strings.Contains(last.Output, "skipped") {
		t.Errorf("last gate = %+v, want g5_eval skipped（deferred 透明化）", last)
	}
	if store.run.CriticReport == nil || !store.run.CriticReport.IsSafe {
		t.Error("critic report not persisted")
	}
	if store.run.Governance == nil || store.run.Governance.RiskLevel != RiskLevelMedium || store.run.Governance.Channel != "notify" {
		t.Errorf("governance = %+v, want medium/notify", store.run.Governance)
	}
	if store.run.RiskLevel != RiskLevelMedium {
		t.Errorf("run.RiskLevel = %q, want medium", store.run.RiskLevel)
	}
	if sandbox.cleanupCalls != 1 {
		t.Errorf("cleanup calls = %d, want 1", sandbox.cleanupCalls)
	}
	if len(*patchReqs) != 1 || (*patchReqs)[0].Attempt != 1 || (*patchReqs)[0].RetryHint != "" {
		t.Errorf("unexpected patch requests: %+v", *patchReqs)
	}
	if store.run.WorktreePath == "" || store.run.Branch != "self-improve/run-1" {
		t.Errorf("worktree/branch not recorded: %+v", store.run)
	}
}

func TestSIPipeline_LowConfidenceRecordOnly(t *testing.T) {
	uc, store, _, patchReqs := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Analyst = siAnalystFn(func(context.Context, *SelfImprovementRun, *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			return &Diagnosis{RootCause: "unsure", ImpactScope: "global", FixStrategy: "investigate", Confidence: 0.3}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusClosed {
		t.Fatalf("status = %s, want closed", store.run.Status)
	}
	if len(*patchReqs) != 0 {
		t.Error("patcher must not be called for record_only")
	}
	if !strings.Contains(store.run.ClosedReason, "confidence") {
		t.Errorf("ClosedReason = %q, want mention confidence", store.run.ClosedReason)
	}
}

func TestSIPipeline_VerifyRetryThenPass(t *testing.T) {
	uc, store, sandbox, patchReqs := siPipelineFixture(3, nil)
	g2Calls := 0
	sandbox.gateFn = func(gate SandboxGateKind, _ int) SandboxGateResult {
		if gate == SandboxGateTest {
			g2Calls++
			if g2Calls == 1 {
				return SandboxGateResult{Gate: gate, Passed: false, Output: "TestX failed"}
			}
		}
		return SandboxGateResult{Gate: gate, Passed: true}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s, want awaiting_governance", store.run.Status)
	}
	if len(*patchReqs) != 2 {
		t.Fatalf("patcher calls = %d, want 2", len(*patchReqs))
	}
	if (*patchReqs)[1].RetryHint == "" || !strings.Contains((*patchReqs)[1].RetryHint, "TestX failed") {
		t.Errorf("retry hint missing gate output: %q", (*patchReqs)[1].RetryHint)
	}
	if store.run.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", store.run.Attempts)
	}
	// 重试回路：verifying 失败后回 patching，再进 verifying
	want := "detected>diagnosing diagnosing>patching patching>verifying verifying>patching patching>verifying verifying>awaiting_governance"
	if got := transitionSeq(store); got != want {
		t.Errorf("transitions = %q, want %q", got, want)
	}
}

func TestSIPipeline_VerifyExhaustsAttempts(t *testing.T) {
	uc, store, sandbox, patchReqs := siPipelineFixture(2, nil)
	sandbox.gateFn = func(gate SandboxGateKind, _ int) SandboxGateResult {
		return SandboxGateResult{Gate: gate, Passed: false, Output: "build broken"}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusVerifyFailed {
		t.Fatalf("status = %s, want verify_failed", store.run.Status)
	}
	if len(*patchReqs) != 2 {
		t.Errorf("patcher calls = %d, want 2 (= maxAttempts)", len(*patchReqs))
	}
	if store.run.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", store.run.Attempts)
	}
}

func TestSIPipeline_ProtectedFileRejectedBeforeGates(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("Makefile", 3), Kind: PatchKindConfig}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Fatalf("status = %s, want rejected", store.run.Status)
	}
	if len(sandbox.gateCalls) != 0 {
		t.Error("gates must not run for protected file")
	}
	if len(sandbox.appliedDiffs) != 0 {
		t.Error("ApplyDiff must not run for protected file")
	}
	if !strings.Contains(store.run.ClosedReason, "protected") {
		t.Errorf("ClosedReason = %q", store.run.ClosedReason)
	}
}

func TestSIPipeline_SensitiveContentRejected(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 2) + "+key = \"sk-abcdefghijklmnop\"\n", Kind: PatchKindCode}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Fatalf("status = %s, want rejected", store.run.Status)
	}
	if len(sandbox.gateCalls) != 0 {
		t.Error("gates must not run for sensitive content")
	}
	if !strings.Contains(store.run.ClosedReason, "sensitive") {
		t.Errorf("ClosedReason = %q", store.run.ClosedReason)
	}
}

func TestSIPipeline_OversizeDiffRejected(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.MaxDiffLines = 10
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 20), Kind: PatchKindCode}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Fatalf("status = %s, want rejected", store.run.Status)
	}
	if !strings.Contains(store.run.ClosedReason, "oversize") {
		t.Errorf("ClosedReason = %q", store.run.ClosedReason)
	}
}

func TestSIPipeline_AnalystErrorFailsRun(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Analyst = siAnalystFn(func(context.Context, *SelfImprovementRun, *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			return nil, errors.New("llm timeout")
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err == nil {
		t.Fatal("Execute must return the stage error")
	}
	if store.run.Status != RunStatusFailed {
		t.Fatalf("status = %s, want failed", store.run.Status)
	}
	if !strings.Contains(store.run.ClosedReason, "llm timeout") {
		t.Errorf("ClosedReason = %q", store.run.ClosedReason)
	}
}

func TestSIPipeline_NilCriticDegrades(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Critic = nil // G4 降级（如配额耗尽）
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s", store.run.Status)
	}
	if store.run.CriticReport != nil {
		t.Error("critic report must stay nil when stage degraded")
	}
	if store.run.Governance == nil {
		t.Fatal("governance must still be computed without critic")
	}
	for _, h := range store.run.Governance.RuleHits {
		if h == "R4" {
			t.Error("R4 must not fire without critic report")
		}
	}
}

func TestSIPipeline_EntryGuard(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, nil)
	store.run.Status = RunStatusApplied // 非 detected 入口
	if err := uc.Execute(context.Background(), "run-1"); err == nil {
		t.Fatal("Execute on non-detected run must error")
	}
	if err := uc.Execute(context.Background(), "missing"); err == nil {
		t.Fatal("Execute on missing run must error")
	}
}
