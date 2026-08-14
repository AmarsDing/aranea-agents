package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ── Mocks for GateVerifier ────────────────────────────────────────────────────

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
