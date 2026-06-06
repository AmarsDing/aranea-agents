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

// ── Mock implementations ──────────────────────────────────────────────────────

type mockExperienceReportWriter struct {
	reports []ExperienceReport
	err     error
}

func (m *mockExperienceReportWriter) Create(_ context.Context, report ExperienceReport) error {
	if m.err != nil {
		return m.err
	}
	m.reports = append(m.reports, report)
	return nil
}

func (m *mockExperienceReportWriter) BatchCreate(_ context.Context, reports []ExperienceReport) error {
	if m.err != nil {
		return m.err
	}
	m.reports = append(m.reports, reports...)
	return nil
}

type mockExperienceReportReader struct {
	reports []ExperienceReport
	err     error
}

func (m *mockExperienceReportReader) ListBySkill(_ context.Context, _ string, _, _ int) ([]ExperienceReport, error) {
	return m.reports, m.err
}

func (m *mockExperienceReportReader) GetByID(_ context.Context, _ string) (*ExperienceReport, error) {
	if len(m.reports) == 0 {
		return nil, m.err
	}
	return &m.reports[0], m.err
}

func (m *mockExperienceReportReader) ListByTimeRange(_ context.Context, _, _ time.Time, _, _ int) ([]ExperienceReport, error) {
	return m.reports, m.err
}

func (m *mockExperienceReportReader) ListFiltered(_ context.Context, _ string, _, _ *time.Time, _, _ int) ([]ExperienceReport, int, error) {
	return m.reports, len(m.reports), m.err
}

type mockSkillHealthAggregator struct {
	metrics   *SkillHealthMetrics
	tagCounts []FailureTagCount
	err       error
}

func (m *mockSkillHealthAggregator) GetHealthMetrics(_ context.Context, _ string, _ time.Time) (*SkillHealthMetrics, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metrics, nil
}

func (m *mockSkillHealthAggregator) GetFailureStats(_ context.Context, _ string, _ time.Time) (*SkillFailureStats, error) {
	return nil, nil
}

func (m *mockSkillHealthAggregator) GetFailureTagCounts(_ context.Context, _ string, _ time.Time) ([]FailureTagCount, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tagCounts, nil
}

type mockSkillEvolutionSuggestionReader struct {
	suggestions []SkillEvolutionSuggestion
	latest      *SkillEvolutionSuggestion
	err         error
}

func (m *mockSkillEvolutionSuggestionReader) ListBySkill(_ context.Context, _ string, _ EvolutionSuggestionStatus, _, _ int) ([]SkillEvolutionSuggestion, error) {
	return m.suggestions, m.err
}

func (m *mockSkillEvolutionSuggestionReader) GetByID(_ context.Context, _ string) (*SkillEvolutionSuggestion, error) {
	if len(m.suggestions) == 0 {
		return nil, m.err
	}
	return &m.suggestions[0], m.err
}

func (m *mockSkillEvolutionSuggestionReader) ListPending(_ context.Context, _, _ int) ([]SkillEvolutionSuggestion, error) {
	return m.suggestions, m.err
}

func (m *mockSkillEvolutionSuggestionReader) GetLatestBySkill(_ context.Context, _ string) (*SkillEvolutionSuggestion, error) {
	return m.latest, m.err
}

func (m *mockSkillEvolutionSuggestionReader) CountBySkill(_ context.Context, _ string, _ EvolutionSuggestionStatus) (int, error) {
	return len(m.suggestions), m.err
}

type mockSkillEvolutionSuggestionWriter struct {
	suggestions []SkillEvolutionSuggestion
	err         error
}

func (m *mockSkillEvolutionSuggestionWriter) Create(_ context.Context, s SkillEvolutionSuggestion) error {
	if m.err != nil {
		return m.err
	}
	m.suggestions = append(m.suggestions, s)
	return nil
}

func (m *mockSkillEvolutionSuggestionWriter) UpdateStatus(_ context.Context, _ string, _ EvolutionSuggestionStatus, _, _ string) error {
	return m.err
}

func (m *mockSkillEvolutionSuggestionWriter) UpdateDraftBody(_ context.Context, _, _ string) error {
	return m.err
}

func (m *mockSkillEvolutionSuggestionWriter) UpdateSandboxResult(_ context.Context, _ string, _ bool, _ json.RawMessage) error {
	return m.err
}

func (m *mockSkillEvolutionSuggestionWriter) UpdateLifecycleStatus(_ context.Context, _ string, _ EvolutionLifecycleStatus) error {
	return m.err
}

type mockRootCauseAnalyzer struct {
	result *RootCauseAnalysisResult
	err    error
}

func (m *mockRootCauseAnalyzer) AnalyzeInvocationFailure(_ context.Context, _ SkillInvocationWrite) (*RootCauseAnalysisResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// newTestUsecase creates a SkillIntelligenceUsecase with the given mocks.
func newTestUsecase(
	writer *mockExperienceReportWriter,
	reader *mockExperienceReportReader,
	aggregator *mockSkillHealthAggregator,
	analyzer ...RootCauseAnalyzer,
) *SkillIntelligenceUsecase {
	lg := loggateway.NewNoop()
	var a RootCauseAnalyzer
	if len(analyzer) > 0 {
		a = analyzer[0]
	}
	return NewSkillIntelligenceUsecase(
		reader,
		writer,
		nil,
		aggregator,
		&mockSkillEvolutionSuggestionReader{},
		&mockSkillEvolutionSuggestionWriter{},
		a,
		lg,
	)
}

// ── TestAnalyzeInvocation ─────────────────────────────────────────────────────

func TestSkillIntelligence_AnalyzeInvocation_Success(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:    "skill-1",
		Outcome:    "success",
		DurationMS: 1000,
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if !isSuccess {
		t.Error("expected isSuccess=true for success outcome")
	}
	if failureTags != nil {
		t.Errorf("expected nil failureTags for success, got %v", failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_Timeout(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   35000, // > TimeoutThresholdMS
		ErrorCode:    "TIMEOUT",
		ErrorMessage: "operation timed out",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for timeout failure")
	}
	if !containsTag(failureTags, FailureTagToolTimeout) {
		t.Errorf("expected tag %q in %v", FailureTagToolTimeout, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_APIError(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   500,
		ErrorCode:    "api_error",
		ErrorMessage: "API returned error",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for API error")
	}
	if !containsTag(failureTags, FailureTagToolAPIError) {
		t.Errorf("expected tag %q in %v", FailureTagToolAPIError, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_ParamMismatch(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   500,
		ErrorCode:    "invalid_param",
		ErrorMessage: "parameter validation failed",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for param mismatch")
	}
	if !containsTag(failureTags, FailureTagParamMismatch) {
		t.Errorf("expected tag %q in %v", FailureTagParamMismatch, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_ContextOverflow(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   500,
		InputPreview: strings.Repeat("x", 6000), // > ContextOverflowThreshold
		ErrorMessage: "context too long",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for context overflow")
	}
	if !containsTag(failureTags, FailureTagContextOverflow) {
		t.Errorf("expected tag %q in %v", FailureTagContextOverflow, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_WrongToolChoice(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   500,
		ErrorMessage: "wrong tool was selected for this task",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for wrong tool choice")
	}
	if !containsTag(failureTags, FailureTagWrongToolChoice) {
		t.Errorf("expected tag %q in %v", FailureTagWrongToolChoice, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_InstructionAmbiguity(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		Outcome:      "failure",
		DurationMS:   500,
		ErrorMessage: "ambiguous instruction provided",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for instruction ambiguity")
	}
	if !containsTag(failureTags, FailureTagInstructionAmbiguity) {
		t.Errorf("expected tag %q in %v", FailureTagInstructionAmbiguity, failureTags)
	}
}

func TestSkillIntelligence_AnalyzeInvocation_UnknownFailure(t *testing.T) {
	uc := newTestUsecase(nil, nil, nil)
	inv := SkillInvocationWrite{
		SkillID: "skill-1",
		Outcome: "failure",
	}
	isSuccess, failureTags := uc.AnalyzeInvocation(context.Background(), inv)
	if isSuccess {
		t.Error("expected isSuccess=false for unknown failure")
	}
	if !containsTag(failureTags, FailureTagUnknown) {
		t.Errorf("expected tag %q in %v", FailureTagUnknown, failureTags)
	}
}

// ── TestScoreSkill ────────────────────────────────────────────────────────────

func TestSkillIntelligence_ScoreSkill_SufficientData(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 100,
			SuccessCount:    80,
			SuccessRate:     0.8,
			AvgDurationMS:   5000,
		},
	}
	uc := newTestUsecase(nil, nil, agg)

	score, err := uc.ScoreSkill(context.Background(), "skill-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With successRate=0.8, durationFactor=1-5000/30000≈0.833, tokenFactor=0.5, feedbackFactor=0.5
	// score = (0.4*0.8 + 0.25*0.833 + 0.2*0.5 + 0.15*0.5) / 1.0 * 100 ≈ 70
	if score <= 0 || score > 100 {
		t.Errorf("score %d out of valid range [1, 100]", score)
	}
	if score == DefaultNeutralScore {
		t.Errorf("score should not be default neutral %d when data is sufficient", DefaultNeutralScore)
	}
}

func TestSkillIntelligence_ScoreSkill_InsufficientData(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 3, // < MinInvocationCount
			SuccessCount:    2,
			SuccessRate:     0.66,
			AvgDurationMS:   5000,
		},
	}
	uc := newTestUsecase(nil, nil, agg)

	score, err := uc.ScoreSkill(context.Background(), "skill-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != DefaultNeutralScore {
		t.Errorf("expected default neutral score %d for insufficient data, got %d", DefaultNeutralScore, score)
	}
}

func TestSkillIntelligence_ScoreSkill_AggregatorError(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		err: fmt.Errorf("aggregator unavailable"),
	}
	uc := newTestUsecase(nil, nil, agg)

	_, err := uc.ScoreSkill(context.Background(), "skill-1")
	if err == nil {
		t.Error("expected error when aggregator returns error")
	}
}

// ── TestGenerateReport ────────────────────────────────────────────────────────

func TestSkillIntelligence_GenerateReport_SuccessInvocation(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	uc := newTestUsecase(writer, nil, agg)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-1",
		Outcome:      "success",
		DurationMS:   2000,
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.IsSuccess {
		t.Error("expected IsSuccess=true for success invocation")
	}
	if len(report.FailureTags) != 0 {
		t.Errorf("expected no failure tags for success, got %v", report.FailureTags)
	}
	if report.FlowSummary == "" {
		t.Error("expected non-empty FlowSummary")
	}
	if report.SkillID != "skill-1" {
		t.Errorf("expected SkillID=skill-1, got %q", report.SkillID)
	}
	if len(writer.reports) != 1 {
		t.Errorf("expected 1 persisted report, got %d", len(writer.reports))
	}
}

func TestSkillIntelligence_GenerateReport_FailureInvocation(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	uc := newTestUsecase(writer, nil, agg)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-2",
		Outcome:      "failure",
		DurationMS:   35000,
		ErrorCode:    "api_error",
		ErrorMessage: "API call failed with 500",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.IsSuccess {
		t.Error("expected IsSuccess=false for failure invocation")
	}
	if len(report.FailureTags) == 0 {
		t.Error("expected failure tags for failure invocation")
	}
	if report.OptimizationAdvice == "" {
		t.Error("expected non-empty OptimizationAdvice for failure invocation")
	}
}

func TestSkillIntelligence_GenerateReport_NilWriter(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	uc := newTestUsecase(nil, nil, agg)
	uc.writer = nil // explicitly nil writer

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-3",
		Outcome:      "success",
		DurationMS:   1000,
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report even when writer is nil")
	}
	if report.SkillID != "skill-1" {
		t.Errorf("expected SkillID=skill-1, got %q", report.SkillID)
	}
}

// ── TestGenerateReport with RootCauseAnalysis ─────────────────────────────────

func TestSkillIntelligence_GenerateReport_RootCauseAnalysis_Populated(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	analyzer := &mockRootCauseAnalyzer{
		result: &RootCauseAnalysisResult{
			RootCause:  "LLM provider response time exceeded configured timeout",
			FixSuggest: "Increase provider timeout or switch to a faster model",
			Severity:   "high",
			Confidence: 0.8,
		},
	}
	uc := newTestUsecase(writer, nil, agg, analyzer)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-rca-1",
		Outcome:      "failure",
		DurationMS:   35000,
		ErrorCode:    "TIMEOUT",
		ErrorMessage: "operation timed out",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.RootCauseAnalysis == "" {
		t.Error("expected non-empty RootCauseAnalysis for failed invocation with analyzer")
	}
	if report.RootCauseAnalysis != "LLM provider response time exceeded configured timeout" {
		t.Errorf("expected specific RootCauseAnalysis, got %q", report.RootCauseAnalysis)
	}
	if report.SuggestedFix == "" {
		t.Error("expected non-empty SuggestedFix for failed invocation with analyzer")
	}
	if report.SuggestedFix != "Increase provider timeout or switch to a faster model" {
		t.Errorf("expected specific SuggestedFix, got %q", report.SuggestedFix)
	}
}

func TestSkillIntelligence_GenerateReport_RootCauseAnalysis_SuccessNoRCA(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	analyzer := &mockRootCauseAnalyzer{
		result: &RootCauseAnalysisResult{
			RootCause:  "should not be used",
			FixSuggest: "should not be used",
		},
	}
	uc := newTestUsecase(writer, nil, agg, analyzer)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-rca-2",
		Outcome:      "success",
		DurationMS:   1000,
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.RootCauseAnalysis != "" {
		t.Errorf("expected empty RootCauseAnalysis for success invocation, got %q", report.RootCauseAnalysis)
	}
	if report.SuggestedFix != "" {
		t.Errorf("expected empty SuggestedFix for success invocation, got %q", report.SuggestedFix)
	}
}

func TestSkillIntelligence_GenerateReport_RootCauseAnalysis_NilAnalyzer(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	uc := newTestUsecase(writer, nil, agg) // no analyzer

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-rca-3",
		Outcome:      "failure",
		DurationMS:   35000,
		ErrorCode:    "TIMEOUT",
		ErrorMessage: "operation timed out",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.RootCauseAnalysis != "" {
		t.Errorf("expected empty RootCauseAnalysis when analyzer is nil, got %q", report.RootCauseAnalysis)
	}
	if report.SuggestedFix != "" {
		t.Errorf("expected empty SuggestedFix when analyzer is nil, got %q", report.SuggestedFix)
	}
}

func TestSkillIntelligence_GenerateReport_RootCauseAnalysis_AnalyzerReturnsNil(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	analyzer := &mockRootCauseAnalyzer{
		result: nil, // no rule matches
	}
	uc := newTestUsecase(writer, nil, agg, analyzer)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-rca-4",
		Outcome:      "failure",
		DurationMS:   500,
		ErrorMessage: "something went wrong",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.RootCauseAnalysis != "" {
		t.Errorf("expected empty RootCauseAnalysis when analyzer returns nil, got %q", report.RootCauseAnalysis)
	}
	if report.SuggestedFix != "" {
		t.Errorf("expected empty SuggestedFix when analyzer returns nil, got %q", report.SuggestedFix)
	}
}

func TestSkillIntelligence_GenerateReport_RootCauseAnalysis_AnalyzerError(t *testing.T) {
	writer := &mockExperienceReportWriter{}
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-1",
			InvocationCount: 50,
			SuccessCount:    45,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
	}
	analyzer := &mockRootCauseAnalyzer{
		err: fmt.Errorf("analyzer unavailable"),
	}
	uc := newTestUsecase(writer, nil, agg, analyzer)

	inv := SkillInvocationWrite{
		SkillID:      "skill-1",
		SessionID:    "session-1",
		ActivationID: "activation-rca-5",
		Outcome:      "failure",
		DurationMS:   35000,
		ErrorCode:    "TIMEOUT",
		ErrorMessage: "operation timed out",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v (analyzer error should be non-fatal)", err)
	}
	if report.RootCauseAnalysis != "" {
		t.Errorf("expected empty RootCauseAnalysis when analyzer errors, got %q", report.RootCauseAnalysis)
	}
	if report.SuggestedFix != "" {
		t.Errorf("expected empty SuggestedFix when analyzer errors, got %q", report.SuggestedFix)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ── Test CheckEvolutionTriggers: 7d success rate < 60% ─────────────────────────

func TestCheckEvolutionTriggers_7dLowSuccessRate(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-7d-bad",
			InvocationCount: 10,
			SuccessCount:    3,
			SuccessRate:     0.3, // < 60%
			AvgDurationMS:   5000,
		},
		tagCounts: []FailureTagCount{},
	}
	sugWriter := &mockSkillEvolutionSuggestionWriter{}
	sugReader := &mockSkillEvolutionSuggestionReader{}
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, agg, sugReader, sugWriter, nil, loggateway.NewNoop())

	suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-7d-bad")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected suggestion for 7d low success rate")
	}
	if suggestion.Type != EvoSuggestionFixFailure {
		t.Errorf("expected type fix_failure, got %q", suggestion.Type)
	}
}

// ── Test CheckEvolutionTriggers: same failure tag >= 5 ─────────────────────────

func TestCheckEvolutionTriggers_SameFailureTagThreshold(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-tag-repeat",
			InvocationCount: 20,
			SuccessCount:    15,
			SuccessRate:     0.75, // healthy success rate
			AvgDurationMS:   3000,
		},
		tagCounts: []FailureTagCount{
			{Tag: FailureTagToolTimeout, Count: 6}, // >= 5 threshold
			{Tag: FailureTagToolAPIError, Count: 2},
		},
	}
	sugWriter := &mockSkillEvolutionSuggestionWriter{}
	sugReader := &mockSkillEvolutionSuggestionReader{}
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, agg, sugReader, sugWriter, nil, loggateway.NewNoop())

	suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-tag-repeat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected suggestion for same failure tag >= 5")
	}
	if suggestion.Type != EvoSuggestionFixFailure {
		t.Errorf("expected type fix_failure, got %q", suggestion.Type)
	}
	if !strings.Contains(suggestion.TriggerReason, FailureTagToolTimeout) {
		t.Errorf("expected trigger reason to mention tag %q, got %q", FailureTagToolTimeout, suggestion.TriggerReason)
	}
}

func TestCheckEvolutionTriggers_SameFailureTagBelowThreshold(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-tag-ok",
			InvocationCount: 20,
			SuccessCount:    18,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
		tagCounts: []FailureTagCount{
			{Tag: FailureTagToolTimeout, Count: 3}, // < 5 threshold
		},
	}
	sugWriter := &mockSkillEvolutionSuggestionWriter{}
	sugReader := &mockSkillEvolutionSuggestionReader{}
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, agg, sugReader, sugWriter, nil, loggateway.NewNoop())

	suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-tag-ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion != nil {
		t.Errorf("expected nil suggestion when same tag below threshold, got type=%q", suggestion.Type)
	}
}

// ── Test RunCuratorFlow: full semi-automatic evolution pipeline ────────────────

func TestRunCuratorFlow_Success(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-curator",
			InvocationCount: 20,
			SuccessCount:    5,
			SuccessRate:     0.25,
			AvgDurationMS:   25000,
		},
		tagCounts: []FailureTagCount{},
	}
	sugWriter := &mockSkillEvolutionSuggestionWriter{}
	sugReader := &mockSkillEvolutionSuggestionReader{}
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, agg, sugReader, sugWriter, nil, loggateway.NewNoop())

	suggestion, err := uc.RunCuratorFlow(context.Background(), "skill-curator")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion == nil {
		t.Fatal("expected suggestion from RunCuratorFlow")
	}
	if suggestion.Status != EvoSuggestionPending {
		t.Errorf("expected status=pending, got %q", suggestion.Status)
	}
	if suggestion.LifecycleStatus != EvoLifecycleReady {
		t.Errorf("expected lifecycle_status=ready (sandbox passed), got %q", suggestion.LifecycleStatus)
	}
	if suggestion.DraftSkillBody == "" {
		t.Error("expected non-empty DraftSkillBody after curator flow")
	}
}

func TestRunCuratorFlow_NoTrigger(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-healthy",
			InvocationCount: 20,
			SuccessCount:    18,
			SuccessRate:     0.9,
			AvgDurationMS:   3000,
		},
		tagCounts: []FailureTagCount{},
	}
	sugWriter := &mockSkillEvolutionSuggestionWriter{}
	sugReader := &mockSkillEvolutionSuggestionReader{}
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, agg, sugReader, sugWriter, nil, loggateway.NewNoop())

	suggestion, err := uc.RunCuratorFlow(context.Background(), "skill-healthy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion != nil {
		t.Errorf("expected nil suggestion for healthy skill, got type=%q", suggestion.Type)
	}
}
