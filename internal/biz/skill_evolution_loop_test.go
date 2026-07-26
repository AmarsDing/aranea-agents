package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── Mocks for SkillEvolutionLoop ──────────────────────────────────────────────

type mockSkillTaskRunner struct {
	result *SkillTaskResult
	err    error
}

func (m *mockSkillTaskRunner) RunTask(_ context.Context, _ string, _ string) (*SkillTaskResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockSkillObserver struct {
	report *EvolutionObservationReport
	err    error
}

func (m *mockSkillObserver) Observe(_ context.Context, _ string, _ *SkillTaskResult) (*EvolutionObservationReport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.report, nil
}

type mockSkillEvolver struct {
	draft string
	err   error
}

func (m *mockSkillEvolver) Evolve(_ context.Context, _ string, _ *EvolutionObservationReport) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.draft, nil
}

type mockSkillGateVerifier struct {
	result *GateVerificationResult
	err    error
}

func (m *mockSkillGateVerifier) Verify(_ context.Context, _ string, _ string, _ *EvolutionObservationReport) (*GateVerificationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockSkillReloader struct {
	err error
}

func (m *mockSkillReloader) Reload(_ context.Context, _ string, _ string, _ string, _ string) error {
	return m.err
}

// ── Test: Five-stage evolution loop (happy path) ──────────────────────────────

func TestEvolutionLoop_FiveStages_HappyPath(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{
			Success:      true,
			DurationMS:   1500,
			TokenUsage:   500,
			Output:       "task completed",
			ErrorMessage: "",
		},
	}
	observer := &mockSkillObserver{
		report: &EvolutionObservationReport{
			SuccessRate:        0.85,
			AvgDurationMS:      1500,
			AvgTokenUsage:      500,
			InvocationCount:    20,
			FailureTagCounts:   map[string]int{},
			StructuredLogs:     []string{"invocation ok"},
			PerformanceMetrics: map[string]float64{"latency_p95": 2000},
		},
	}
	evolver := &mockSkillEvolver{
		draft: "# Evolved Skill\n\nImproved version with better error handling.",
	}
	gate := &mockSkillGateVerifier{
		result: &GateVerificationResult{
			Passed: true,
			Checks: []GateCheckResult{
				{Name: "functional", Passed: true},
				{Name: "security", Passed: true},
				{Name: "performance", Passed: true},
				{Name: "style", Passed: true},
			},
		},
	}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected loop to pass")
	}
	if result.Stage != EvoStageReload {
		t.Errorf("expected final stage=%q, got %q", EvoStageReload, result.Stage)
	}
	if result.DraftBody == "" {
		t.Error("expected non-empty draft body")
	}
}

// ── Test: Gate rejection stops the loop ────────────────────────────────────────

func TestEvolutionLoop_GateRejection_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500},
	}
	observer := &mockSkillObserver{
		report: &EvolutionObservationReport{
			SuccessRate:     0.85,
			AvgDurationMS:   1500,
			AvgTokenUsage:   500,
			InvocationCount: 20,
		},
	}
	evolver := &mockSkillEvolver{
		draft: "# Evolved Skill\n\nContains API key: sk-abc123def456xyz789mno012",
	}
	gate := &mockSkillGateVerifier{
		result: &GateVerificationResult{
			Passed: false,
			Checks: []GateCheckResult{
				{Name: "functional", Passed: true},
				{Name: "security", Passed: false, Reason: "sensitive info detected: API key pattern"},
				{Name: "performance", Passed: true},
				{Name: "style", Passed: true},
			},
		},
	}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Gate stage")
	}
	if result.Stage != EvoStageGate {
		t.Errorf("expected stage=%q, got %q", EvoStageGate, result.Stage)
	}
}

// ── Test: Solve failure stops the loop ─────────────────────────────────────────

func TestEvolutionLoop_SolveFailure_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		err: fmt.Errorf("task execution failed"),
	}
	observer := &mockSkillObserver{}
	evolver := &mockSkillEvolver{}
	gate := &mockSkillGateVerifier{}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Solve stage")
	}
	if result.Stage != EvoStageSolve {
		t.Errorf("expected stage=%q, got %q", EvoStageSolve, result.Stage)
	}
}

// ── Test: Evolve failure stops the loop ────────────────────────────────────────

func TestEvolutionLoop_EvolveFailure_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500},
	}
	observer := &mockSkillObserver{
		report: &EvolutionObservationReport{SuccessRate: 0.85, InvocationCount: 20},
	}
	evolver := &mockSkillEvolver{
		err: fmt.Errorf("evolution generation failed"),
	}
	gate := &mockSkillGateVerifier{}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Evolve stage")
	}
	if result.Stage != EvoStageEvolve {
		t.Errorf("expected stage=%q, got %q", EvoStageEvolve, result.Stage)
	}
}

// ── Test: Gate multi-dimensional verification ──────────────────────────────────

func TestGateVerification_AllDimensionsPass(t *testing.T) {
	verifier := NewGateVerifier(nil, nil) // nil sandbox runner and lint checker → use rule-based fallback

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nNo sensitive data.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected all checks to pass for clean draft")
	}
	for _, check := range result.Checks {
		if !check.Passed {
			t.Errorf("expected check %q to pass, got reason: %s", check.Name, check.Reason)
		}
	}
}

func TestGateVerification_SecurityDetectsAPIKey(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	draftWithAPIKey := "# Skill\n\nUse this key: sk-abc123def456xyz789mno012"
	result, err := verifier.Verify(context.Background(), "skill-1", draftWithAPIKey, observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected security check to fail for draft with API key")
	}
	securityPassed := true
	for _, check := range result.Checks {
		if check.Name == "security" {
			securityPassed = check.Passed
		}
	}
	if securityPassed {
		t.Error("expected security dimension to fail for API key pattern")
	}
}

func TestGateVerification_SecurityDetectsPassword(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	draftWithPassword := "# Skill\n\npassword: MyS3cretP@ss!"
	result, err := verifier.Verify(context.Background(), "skill-1", draftWithPassword, observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected security check to fail for draft with password")
	}
}

func TestGateVerification_SecurityDetectsToken(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	draftWithToken := "# Skill\n\ntoken: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc.def"
	result, err := verifier.Verify(context.Background(), "skill-1", draftWithToken, observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected security check to fail for draft with token")
	}
}

func TestGateVerification_PerformanceDegradation(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Baseline: 1500ms avg, 500 tokens. New: 2000ms avg, 700 tokens.
	// Duration degradation: (2000-1500)/1500 = 33% > 20% → reject
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      2000, // 33% worse than 1500ms baseline
		AvgTokenUsage:      700,  // 40% worse than 500 token baseline
		InvocationCount:    20,
		BaselineDurationMS: 1500,
		BaselineTokenUsage: 500,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected performance check to fail for >20% degradation")
	}
}

func TestGateVerification_PerformanceWithinThreshold(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Baseline: 1500ms, 500 tokens. New: 1700ms, 550 tokens.
	// Duration: (1700-1500)/1500 = 13% < 20% → OK
	// Token: (550-500)/500 = 10% < 20% → OK
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      1700,
		AvgTokenUsage:      550,
		InvocationCount:    20,
		BaselineDurationMS: 1500,
		BaselineTokenUsage: 500,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected performance check to pass for <20% degradation")
	}
}

func TestGateVerification_FunctionalCheck_EmptyDraft(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected functional check to fail for empty draft")
	}
}

func TestGateVerification_StyleCheck_DraftTooLong(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	longDraft := "# Skill\n\n" + strings.Repeat("x", 15000) // > 10000 chars
	result, err := verifier.Verify(context.Background(), "skill-1", longDraft, observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected style check to fail for overly long draft")
	}
}

// ── Test: Evolution suggestion expiration ──────────────────────────────────────

func TestEvolutionLoop_ExpirePendingSuggestions(t *testing.T) {
	// Create a suggestion that is 8 days old (past 7-day expiration)
	oldTime := time.Now().UTC().Add(-8 * 24 * time.Hour)
	oldSuggestion := SkillEvolutionSuggestion{
		ID:              "sug-old-1",
		SkillID:         "skill-1",
		Status:          EvoSuggestionPending,
		LifecycleStatus: EvoLifecycleDraft,
		CreatedAt:       oldTime,
	}

	// Create a recent suggestion that should NOT be expired
	recentSuggestion := SkillEvolutionSuggestion{
		ID:              "sug-recent-1",
		SkillID:         "skill-2",
		Status:          EvoSuggestionPending,
		LifecycleStatus: EvoLifecycleDraft,
		CreatedAt:       time.Now().UTC().Add(-3 * 24 * time.Hour),
	}

	uc := NewSkillIntelligenceUsecase(nil, nil, &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{oldSuggestion, recentSuggestion}}, nil, loggateway.NewNoop())

	expired, err := uc.ExpirePendingSuggestions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired suggestion, got %d", len(expired))
	}
	if expired[0].ID != "sug-old-1" {
		t.Errorf("expected expired suggestion ID=sug-old-1, got %q", expired[0].ID)
	}
}

func TestEvolutionLoop_ExpirePendingSuggestions_NoneExpired(t *testing.T) {
	// All suggestions are recent
	uc := NewSkillIntelligenceUsecase(nil, nil, &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{
		{
			ID:              "sug-1",
			SkillID:         "skill-1",
			Status:          EvoSuggestionPending,
			LifecycleStatus: EvoLifecycleDraft,
			CreatedAt:       time.Now().UTC().Add(-2 * 24 * time.Hour),
		},
	}}, nil, loggateway.NewNoop())

	expired, err := uc.ExpirePendingSuggestions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired suggestions, got %d", len(expired))
	}
}

func TestEvolutionLoop_ExpirePendingSuggestions_OnlyPendingExpired(t *testing.T) {
	oldTime := time.Now().UTC().Add(-8 * 24 * time.Hour)

	// Approved suggestion should NOT be expired even if old
	uc := NewSkillIntelligenceUsecase(nil, nil, &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{
		{
			ID:              "sug-approved-old",
			SkillID:         "skill-1",
			Status:          EvoSuggestionApproved,
			LifecycleStatus: EvoLifecycleReady,
			CreatedAt:       oldTime,
		},
	}}, nil, loggateway.NewNoop())

	expired, err := uc.ExpirePendingSuggestions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired (approved should not be expired), got %d", len(expired))
	}
}

// ── Test: Reload stage sets parent_version_id and evolution_reason ─────────────

func TestEvolutionLoop_Reload_SetsParentVersionAndReason(t *testing.T) {
	var capturedSkillID, capturedParentVer, capturedReason string

	reloader := &mockSkillReloaderWithCapture{
		capture: func(skillID, _draft, parentVer, reason string) {
			capturedSkillID = skillID
			capturedParentVer = parentVer
			capturedReason = reason
		},
	}

	loop := NewSkillEvolutionLoop(
		&mockSkillTaskRunner{result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500}},
		&mockSkillObserver{report: &EvolutionObservationReport{SuccessRate: 0.85, InvocationCount: 20}},
		&mockSkillEvolver{draft: "# Evolved Skill"},
		&mockSkillGateVerifier{result: &GateVerificationResult{Passed: true, Checks: []GateCheckResult{
			{Name: "functional", Passed: true},
			{Name: "security", Passed: true},
			{Name: "performance", Passed: true},
			{Name: "style", Passed: true},
		}}},
		reloader,
		loggateway.NewNoop(),
	)

	result, err := loop.Run(context.Background(), "skill-1", "test task", EvolutionLoopOptions{CurrentVersionID: "ver-001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected loop to pass")
	}
	if capturedSkillID != "skill-1" {
		t.Errorf("expected skillID=skill-1, got %q", capturedSkillID)
	}
	if capturedParentVer != "ver-001" {
		t.Errorf("expected parentVersionID=ver-001, got %q", capturedParentVer)
	}
	if !strings.Contains(capturedReason, "evolution") {
		t.Errorf("expected evolution_reason to contain 'evolution', got %q", capturedReason)
	}
}

// ── Test: Observation stage collects structured data ───────────────────────────

func TestEvolutionLoop_ObserveStage_CollectsStructuredData(t *testing.T) {
	var capturedResult *SkillTaskResult
	var capturedSkillID string

	observer := &mockSkillObserverWithCapture{
		capture: func(skillID string, result *SkillTaskResult) {
			capturedSkillID = skillID
			capturedResult = result
		},
		report: &EvolutionObservationReport{
			SuccessRate:     0.85,
			AvgDurationMS:   1500,
			AvgTokenUsage:   500,
			InvocationCount: 20,
		},
	}

	loop := NewSkillEvolutionLoop(
		&mockSkillTaskRunner{result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500, Output: "done"}},
		observer,
		&mockSkillEvolver{draft: "# Evolved Skill"},
		&mockSkillGateVerifier{result: &GateVerificationResult{Passed: true}},
		&mockSkillReloader{},
		loggateway.NewNoop(),
	)

	_, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSkillID != "skill-1" {
		t.Errorf("expected skillID=skill-1, got %q", capturedSkillID)
	}
	if capturedResult == nil || !capturedResult.Success {
		t.Error("expected captured result to be success")
	}
}

// ── Helper mocks with capture ─────────────────────────────────────────────────

type mockSkillReloaderWithCapture struct {
	capture func(skillID, draft, parentVer, reason string)
}

func (m *mockSkillReloaderWithCapture) Reload(_ context.Context, skillID, draft, parentVer, reason string) error {
	if m.capture != nil {
		m.capture(skillID, draft, parentVer, reason)
	}
	return nil
}

type mockSkillObserverWithCapture struct {
	report  *EvolutionObservationReport
	err     error
	capture func(skillID string, result *SkillTaskResult)
}

func (m *mockSkillObserverWithCapture) Observe(_ context.Context, skillID string, result *SkillTaskResult) (*EvolutionObservationReport, error) {
	if m.capture != nil {
		m.capture(skillID, result)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.report, nil
}

// ── Test: GateVerifier with SandboxRunner ──────────────────────────────────────

func TestGateVerification_WithSandboxRunner(t *testing.T) {
	sandboxRunner := &mockSandboxRunnerForGate{passed: true}
	verifier := NewGateVerifier(sandboxRunner, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected all checks to pass with sandbox runner")
	}
	// Verify functional check used sandbox runner
	funcPassed := false
	for _, check := range result.Checks {
		if check.Name == "functional" {
			funcPassed = check.Passed
		}
	}
	if !funcPassed {
		t.Error("expected functional check to pass with sandbox runner")
	}
}

func TestGateVerification_SandboxRunnerFails(t *testing.T) {
	sandboxRunner := &mockSandboxRunnerForGate{passed: false}
	verifier := NewGateVerifier(sandboxRunner, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected gate to fail when sandbox runner fails")
	}
}

type mockSandboxRunnerForGate struct {
	passed bool
	err    error
}

func (m *mockSandboxRunnerForGate) RunSandbox(_ context.Context, _ string, _ string) (bool, json.RawMessage, error) {
	if m.err != nil {
		return false, nil, m.err
	}
	resultJSON, _ := json.Marshal(map[string]any{"passed": m.passed})
	return m.passed, resultJSON, nil
}

type mockSkillLintChecker struct {
	passed bool
	reason string
	err    error
}

func (m *mockSkillLintChecker) LintCheck(_ context.Context, _ string) (bool, string, error) {
	return m.passed, m.reason, m.err
}

// ── Test: Observe failure stops the loop ────────────────────────────────────────

func TestEvolutionLoop_ObserveFailure_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500},
	}
	observer := &mockSkillObserver{
		err: fmt.Errorf("observation collection failed"),
	}
	evolver := &mockSkillEvolver{}
	gate := &mockSkillGateVerifier{}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Observe stage")
	}
	if result.Stage != EvoStageObserve {
		t.Errorf("expected stage=%q, got %q", EvoStageObserve, result.Stage)
	}
}

// ── Test: Reload failure stops the loop ─────────────────────────────────────────

func TestEvolutionLoop_ReloadFailure_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{Success: true, DurationMS: 1500, TokenUsage: 500},
	}
	observer := &mockSkillObserver{
		report: &EvolutionObservationReport{SuccessRate: 0.85, InvocationCount: 20},
	}
	evolver := &mockSkillEvolver{draft: "# Evolved Skill"}
	gate := &mockSkillGateVerifier{
		result: &GateVerificationResult{Passed: true, Checks: []GateCheckResult{
			{Name: "functional", Passed: true},
			{Name: "security", Passed: true},
			{Name: "performance", Passed: true},
			{Name: "style", Passed: true},
		}},
	}
	reloader := &mockSkillReloader{err: fmt.Errorf("reload registration failed")}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Reload stage")
	}
	if result.Stage != EvoStageReload {
		t.Errorf("expected stage=%q, got %q", EvoStageReload, result.Stage)
	}
}

// ── Test: Empty skillID is rejected ─────────────────────────────────────────────

func TestEvolutionLoop_EmptySkillID_ReturnsError(t *testing.T) {
	loop := NewSkillEvolutionLoop(nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := loop.Run(context.Background(), "", "test task")
	if err == nil {
		t.Fatal("expected error for empty skillID, got nil")
	}
}

// ── Test: Solve stage with Success=false (task failed without error) ────────────

func TestEvolutionLoop_SolveTaskFailed_StopsLoop(t *testing.T) {
	runner := &mockSkillTaskRunner{
		result: &SkillTaskResult{
			Success:      false,
			DurationMS:   1500,
			TokenUsage:   500,
			ErrorMessage: "task output did not match expected",
		},
	}
	observer := &mockSkillObserver{}
	evolver := &mockSkillEvolver{}
	gate := &mockSkillGateVerifier{}
	reloader := &mockSkillReloader{}

	loop := NewSkillEvolutionLoop(runner, observer, evolver, gate, reloader, loggateway.NewNoop())

	result, err := loop.Run(context.Background(), "skill-1", "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected loop to fail at Solve stage when task Success=false")
	}
	if result.Stage != EvoStageSolve {
		t.Errorf("expected stage=%q, got %q", EvoStageSolve, result.Stage)
	}
}

// ── Test: Gate verification - Token-only performance degradation ─────────────────

func TestGateVerification_TokenOnlyDegradation(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Duration is fine (10% degradation), but token usage is 30% worse
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      1650, // 10% worse than 1500ms → OK
		AvgTokenUsage:      650,  // 30% worse than 500 → reject
		InvocationCount:    20,
		BaselineDurationMS: 1500,
		BaselineTokenUsage: 500,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected performance check to fail for token-only >20% degradation")
	}
	// Verify the failing check is specifically about token usage
	var perfCheck *GateCheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "performance" {
			perfCheck = &result.Checks[i]
		}
	}
	if perfCheck == nil {
		t.Fatal("expected performance check to exist")
	}
	if perfCheck.Passed {
		t.Error("expected performance check to fail")
	}
	if !strings.Contains(perfCheck.Reason, "token") {
		t.Errorf("expected reason to mention token degradation, got %q", perfCheck.Reason)
	}
}

// ── Test: Gate verification - Style check without heading ────────────────────────

func TestGateVerification_StyleCheck_NoHeading(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	// Draft without any markdown heading
	draftNoHeading := "This is a skill description without any heading."
	result, err := verifier.Verify(context.Background(), "skill-1", draftNoHeading, observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected style check to fail for draft without heading")
	}
	var styleCheck *GateCheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "style" {
			styleCheck = &result.Checks[i]
		}
	}
	if styleCheck == nil {
		t.Fatal("expected style check to exist")
	}
	if styleCheck.Passed {
		t.Error("expected style check to fail for draft without heading")
	}
}

// ── Test: Gate verification - LintChecker integration ───────────────────────────

func TestGateVerification_WithLintChecker_Pass(t *testing.T) {
	lintChecker := &mockSkillLintChecker{passed: true}
	verifier := NewGateVerifier(nil, lintChecker)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected all checks to pass with lint checker")
	}
}

func TestGateVerification_WithLintChecker_Fail(t *testing.T) {
	lintChecker := &mockSkillLintChecker{passed: false, reason: "missing required section: examples"}
	verifier := NewGateVerifier(nil, lintChecker)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "# Skill", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected gate to fail when lint checker fails")
	}
	var styleCheck *GateCheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "style" {
			styleCheck = &result.Checks[i]
		}
	}
	if styleCheck == nil {
		t.Fatal("expected style check to exist")
	}
	if styleCheck.Passed {
		t.Error("expected style check to fail when lint checker returns false")
	}
	if !strings.Contains(styleCheck.Reason, "missing required section") {
		t.Errorf("expected style reason to contain lint message, got %q", styleCheck.Reason)
	}
}

func TestGateVerification_WithLintChecker_Error(t *testing.T) {
	lintChecker := &mockSkillLintChecker{err: fmt.Errorf("lint service unavailable")}
	verifier := NewGateVerifier(nil, lintChecker)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "# Skill", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected gate to fail when lint checker returns error")
	}
	var styleCheck *GateCheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "style" {
			styleCheck = &result.Checks[i]
		}
	}
	if styleCheck == nil {
		t.Fatal("expected style check to exist")
	}
	if styleCheck.Passed {
		t.Error("expected style check to fail when lint checker errors")
	}
	if !strings.Contains(styleCheck.Reason, "lint check error") {
		t.Errorf("expected style reason to mention lint error, got %q", styleCheck.Reason)
	}
}

// ── Test: Gate verification - SandboxRunner error handling ───────────────────────

func TestGateVerification_SandboxRunnerError(t *testing.T) {
	sandboxRunner := &mockSandboxRunnerForGate{err: fmt.Errorf("sandbox environment unavailable")}
	verifier := NewGateVerifier(sandboxRunner, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected gate to fail when sandbox runner returns error")
	}
	var funcCheck *GateCheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "functional" {
			funcCheck = &result.Checks[i]
		}
	}
	if funcCheck == nil {
		t.Fatal("expected functional check to exist")
	}
	if funcCheck.Passed {
		t.Error("expected functional check to fail when sandbox errors")
	}
	if !strings.Contains(funcCheck.Reason, "sandbox execution error") {
		t.Errorf("expected reason to mention sandbox error, got %q", funcCheck.Reason)
	}
}

// ── Test: Gate verification - Nil observation (performance check passes) ─────────

func TestGateVerification_NilObservation_PerformancePasses(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected all checks to pass with nil observation (no baseline to compare)")
	}
}

// ── Test: Expiration - ListPending error ────────────────────────────────────────

func TestEvolutionLoop_ExpirePendingSuggestions_ListPendingError(t *testing.T) {
	uc := NewSkillIntelligenceUsecase(nil, nil, &mockEvolutionStoreBridge{err: fmt.Errorf("database unavailable")}, nil, loggateway.NewNoop())

	_, err := uc.ExpirePendingSuggestions(context.Background())
	if err == nil {
		t.Fatal("expected error when ListPending fails, got nil")
	}
}

// ── Test: Expiration - Nil reader/writer (no-op) ────────────────────────────────

func TestEvolutionLoop_ExpirePendingSuggestions_NilAccessors(t *testing.T) {
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, nil, loggateway.NewNoop())

	expired, err := uc.ExpirePendingSuggestions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired with nil accessors, got %d", len(expired))
	}
}

// ── Test: Expiration - Partial UpdateStatus failure ─────────────────────────────

func TestEvolutionLoop_ExpirePendingSuggestions_PartialUpdateFailure(t *testing.T) {
	oldTime := time.Now().UTC().Add(-8 * 24 * time.Hour)
	suggestions := []SkillEvolutionSuggestion{
		{ID: "sug-old-1", SkillID: "skill-1", Status: EvoSuggestionPending, LifecycleStatus: EvoLifecycleDraft, CreatedAt: oldTime},
		{ID: "sug-old-2", SkillID: "skill-2", Status: EvoSuggestionPending, LifecycleStatus: EvoLifecycleDraft, CreatedAt: oldTime},
	}

	legacyBridge := &mockEvolutionStoreBridgeWithPartialFailure{
		mockEvolutionStoreBridge: mockEvolutionStoreBridge{suggestions: suggestions},
		failIDs:                  map[string]bool{"sug-old-1": true},
	}

	uc := NewSkillIntelligenceUsecase(nil, nil, legacyBridge, nil, loggateway.NewNoop())

	expired, err := uc.ExpirePendingSuggestions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only sug-old-2 should be in the expired list (sug-old-1 failed to update)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired suggestion (partial failure), got %d", len(expired))
	}
	if expired[0].ID != "sug-old-2" {
		t.Errorf("expected expired ID=sug-old-2, got %q", expired[0].ID)
	}
}

// ── Helper mock for partial UpdateStatus failure ─────────────────────────────

// mockEvolutionStoreBridgeWithPartialFailure wraps mockEvolutionStoreBridge
// with selective UpdateStatus failures based on suggestion ID (A6).
type mockEvolutionStoreBridgeWithPartialFailure struct {
	mockEvolutionStoreBridge
	failIDs map[string]bool
}

func (m *mockEvolutionStoreBridgeWithPartialFailure) UpdateStatus(ctx context.Context, id string, status string, actor string, reason string) error {
	if m.failIDs[id] {
		return fmt.Errorf("update failed for %s", id)
	}
	return m.mockEvolutionStoreBridge.UpdateStatus(ctx, id, status, actor, reason)
}

// ── Test: Gate verification - Functional check with empty skillID ───────────────

func TestGateVerification_FunctionalCheck_EmptySkillID(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	observation := &EvolutionObservationReport{
		SuccessRate:     0.9,
		AvgDurationMS:   1500,
		AvgTokenUsage:   500,
		InvocationCount: 20,
	}

	result, err := verifier.Verify(context.Background(), "", "# Skill Body", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected functional check to fail for empty skill ID")
	}
}

// ── Test: Gate verification - Performance with zero baseline ────────────────────

func TestGateVerification_Performance_ZeroBaseline(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Zero baseline with reasonable current values → should pass (below absolute thresholds)
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      1500,
		AvgTokenUsage:      500,
		InvocationCount:    20,
		BaselineDurationMS: 0,
		BaselineTokenUsage: 0,
	}

	result, err := verifier.Verify(context.Background(), "skill-1", "## Clean Skill\n\nSome content here.", observation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected performance check to pass with zero baseline and reasonable current values")
	}
}

func TestGateVerification_Performance_ZeroBaseline_HighDuration(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Zero baseline but very high current duration → should fail (absolute threshold)
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      90000,
		AvgTokenUsage:      500,
		InvocationCount:    20,
		BaselineDurationMS: 0,
		BaselineTokenUsage: 0,
	}

	perfResult := verifier.verifyPerformance(observation)
	if perfResult.Passed {
		t.Error("expected performance check to fail with zero baseline and high duration (90000ms > 60000ms)")
	}
	if !strings.Contains(perfResult.Reason, "insufficient baseline data") {
		t.Errorf("expected reason to mention insufficient baseline data, got %q", perfResult.Reason)
	}
}

func TestGateVerification_Performance_ZeroBaseline_HighTokenUsage(t *testing.T) {
	verifier := NewGateVerifier(nil, nil)

	// Zero baseline but very high token usage → should fail (absolute threshold)
	observation := &EvolutionObservationReport{
		SuccessRate:        0.9,
		AvgDurationMS:      1500,
		AvgTokenUsage:      15000,
		InvocationCount:    20,
		BaselineDurationMS: 0,
		BaselineTokenUsage: 0,
	}

	perfResult := verifier.verifyPerformance(observation)
	if perfResult.Passed {
		t.Error("expected performance check to fail with zero baseline and high token usage (15000 > 10000)")
	}
	if !strings.Contains(perfResult.Reason, "insufficient baseline data") {
		t.Errorf("expected reason to mention insufficient baseline data, got %q", perfResult.Reason)
	}
}
