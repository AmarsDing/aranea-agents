package biz

import (
	"context"
	"encoding/json"
	"errors"
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

// mockEvolutionStoreBridge implements biz.UnifiedEvolutionStore for testing (A6).
// The store is seeded with legacy-view suggestions; unified-side reads derive
// rows via the same view converters the production code uses, and writes
// mutate the seed slice.
type mockEvolutionStoreBridge struct {
	suggestions []SkillEvolutionSuggestion
	latest      *SkillEvolutionSuggestion
	err         error
}

// unifiedRow converts a seeded legacy suggestion to its unified row form.
func (m *mockEvolutionStoreBridge) unifiedRow(s *SkillEvolutionSuggestion) UnifiedEvolutionSuggestion {
	return unifiedToLegacySuggestion(s)
}

func (m *mockEvolutionStoreBridge) HasPendingForTarget(_ context.Context, targetType string, targetID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	for i := range m.suggestions {
		row := m.unifiedRow(&m.suggestions[i])
		if string(row.TargetType) == targetType && row.TargetID == targetID && row.Status == "pending" {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockEvolutionStoreBridge) GetLatestByTarget(_ context.Context, targetType string, targetID string) (*UnifiedEvolutionSuggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	var best *UnifiedEvolutionSuggestion
	for i := range m.suggestions {
		row := m.unifiedRow(&m.suggestions[i])
		if string(row.TargetType) != targetType || row.TargetID != targetID {
			continue
		}
		if best == nil || row.CreatedAt.After(best.CreatedAt) {
			r := row
			best = &r
		}
	}
	return best, nil
}

func (m *mockEvolutionStoreBridge) GetLatestByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string) (*UnifiedEvolutionSuggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	var best *UnifiedEvolutionSuggestion
	for i := range m.suggestions {
		row := m.unifiedRow(&m.suggestions[i])
		if string(row.TargetType) != targetType || row.TargetID != targetID || string(row.ActionType) != actionType {
			continue
		}
		if best == nil || row.CreatedAt.After(best.CreatedAt) {
			r := row
			best = &r
		}
	}
	return best, nil
}

func (m *mockEvolutionStoreBridge) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	for i := range m.suggestions {
		if m.suggestions[i].ID == id {
			row := m.unifiedRow(&m.suggestions[i])
			return &row, nil
		}
	}
	return nil, nil
}

func (m *mockEvolutionStoreBridge) ListByTarget(_ context.Context, targetType string, targetID string, status string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []UnifiedEvolutionSuggestion
	for i := range m.suggestions {
		row := m.unifiedRow(&m.suggestions[i])
		if string(row.TargetType) != targetType {
			continue
		}
		if targetID != "" && row.TargetID != targetID {
			continue
		}
		if status != "" && row.Status != status {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (m *mockEvolutionStoreBridge) CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error) {
	rows, err := m.ListByTarget(ctx, targetType, targetID, status, 0, 0)
	return len(rows), err
}

func (m *mockEvolutionStoreBridge) ListByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string, status string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []UnifiedEvolutionSuggestion
	for i := range m.suggestions {
		row := m.unifiedRow(&m.suggestions[i])
		if string(row.TargetType) != targetType {
			continue
		}
		if targetID != "" && row.TargetID != targetID {
			continue
		}
		if actionType != "" && string(row.ActionType) != actionType {
			continue
		}
		if status != "" && row.Status != status {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (m *mockEvolutionStoreBridge) CountByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, status string) (int, error) {
	rows, err := m.ListByTargetAndAction(ctx, targetType, targetID, actionType, status, 0, 0)
	return len(rows), err
}

func (m *mockEvolutionStoreBridge) Create(_ context.Context, s UnifiedEvolutionSuggestion) error {
	if m.err != nil {
		return m.err
	}
	m.suggestions = append(m.suggestions, *unifiedToLegacySuggestionPtr(&s))
	return nil
}

func (m *mockEvolutionStoreBridge) UpdateStatus(_ context.Context, id string, status string, actor string, reason string) error {
	if m.err != nil {
		return m.err
	}
	for i := range m.suggestions {
		if m.suggestions[i].ID != id {
			continue
		}
		m.suggestions[i].Status = EvolutionSuggestionStatus(status)
		now := time.Now().UTC()
		m.suggestions[i].ResolvedAt = &now
		switch status {
		case "approved":
			m.suggestions[i].ApprovedBy = actor
		case "rejected":
			m.suggestions[i].RejectedBy = actor
			m.suggestions[i].RejectionReason = reason
		}
		return nil
	}
	return nil
}

func (m *mockEvolutionStoreBridge) UpdateDraftBody(_ context.Context, id string, draft string) error {
	if m.err != nil {
		return m.err
	}
	for i := range m.suggestions {
		if m.suggestions[i].ID == id {
			m.suggestions[i].DraftSkillBody = draft
			return nil
		}
	}
	return nil
}

func (m *mockEvolutionStoreBridge) UpdateLifecycleStatus(_ context.Context, id string, status string) error {
	if m.err != nil {
		return m.err
	}
	for i := range m.suggestions {
		if m.suggestions[i].ID == id {
			m.suggestions[i].LifecycleStatus = EvolutionLifecycleStatus(status)
			return nil
		}
	}
	return nil
}

func (m *mockEvolutionStoreBridge) UpdateSandboxResult(_ context.Context, id string, passed bool, result json.RawMessage) error {
	if m.err != nil {
		return m.err
	}
	for i := range m.suggestions {
		if m.suggestions[i].ID == id {
			m.suggestions[i].SandboxPassed = passed
			m.suggestions[i].SandboxResult = result
			return nil
		}
	}
	return nil
}

func (m *mockEvolutionStoreBridge) UpdateMetadataKey(_ context.Context, _ string, _ string, _ string) error {
	return m.err
}

func (m *mockEvolutionStoreBridge) ExpireOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, m.err
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
	scorer := NewSkillScoringUsecase(aggregator, lg)
	reporter := NewSkillReportUsecase(reader, writer, nil, scorer, a, lg)
	bridge := &mockEvolutionStoreBridge{}
	return NewSkillIntelligenceUsecase(scorer, reporter, bridge, aggregator, lg)
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
	uc.reporter.writer = nil // explicitly nil writer

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
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, &mockEvolutionStoreBridge{}, agg, lg)

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
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, &mockEvolutionStoreBridge{}, agg, lg)

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
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, &mockEvolutionStoreBridge{}, agg, lg)

	suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-tag-ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion != nil {
		t.Errorf("expected nil suggestion when same tag below threshold, got type=%q", suggestion.Type)
	}
}

// ── ValidatePendingSuggestionsForSkill (A1: 验证半管测试) ────────────────────

// recordingUnifiedStore implements UnifiedEvolutionStore with call recording.
type recordingUnifiedStore struct {
	pending         []UnifiedEvolutionSuggestion
	byID            map[string]*UnifiedEvolutionSuggestion // GetByID backing (P0 Reload tests)
	listErr         error
	draftBodyErr    error
	draftBodies     map[string]string
	lifecycleSeq    map[string][]string // suggestion ID → ordered lifecycle transitions
	sandboxPassed   map[string]bool
	sandboxResults  map[string]json.RawMessage
	sandboxWriteErr error
	statusSeq       map[string][]string // suggestion ID → ordered status transitions
	statusErr       error
	metaUpdates     map[string]map[string]string // suggestion ID → metadata key → value
	latest          *UnifiedEvolutionSuggestion   // GetLatestByTarget backing (F9 cooldown tests)
}

func newRecordingUnifiedStore() *recordingUnifiedStore {
	return &recordingUnifiedStore{
		byID:           make(map[string]*UnifiedEvolutionSuggestion),
		draftBodies:    make(map[string]string),
		lifecycleSeq:   make(map[string][]string),
		sandboxPassed:  make(map[string]bool),
		sandboxResults: make(map[string]json.RawMessage),
		statusSeq:      make(map[string][]string),
	}
}

func (s *recordingUnifiedStore) HasPendingForTarget(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *recordingUnifiedStore) GetLatestByTarget(_ context.Context, _, _ string) (*UnifiedEvolutionSuggestion, error) {
	return s.latest, nil
}
func (s *recordingUnifiedStore) GetLatestByTargetAndAction(_ context.Context, _, _, _ string) (*UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *recordingUnifiedStore) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	return s.byID[id], nil
}
func (s *recordingUnifiedStore) ListByTarget(_ context.Context, targetType, targetID, status string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if status != "pending" {
		return nil, nil
	}
	var out []UnifiedEvolutionSuggestion
	for _, p := range s.pending {
		if string(p.TargetType) == targetType && p.TargetID == targetID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *recordingUnifiedStore) CountByTarget(_ context.Context, _, _, _ string) (int, error) {
	return len(s.pending), nil
}
func (s *recordingUnifiedStore) ListByTargetAndAction(_ context.Context, _, _, _, _ string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *recordingUnifiedStore) CountByTargetAndAction(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, nil
}
func (s *recordingUnifiedStore) Create(_ context.Context, _ UnifiedEvolutionSuggestion) error {
	return nil
}
func (s *recordingUnifiedStore) UpdateStatus(_ context.Context, id, status, _, _ string) error {
	if s.statusErr != nil {
		return s.statusErr
	}
	s.statusSeq[id] = append(s.statusSeq[id], status)
	return nil
}
func (s *recordingUnifiedStore) UpdateDraftBody(_ context.Context, id, draft string) error {
	if s.draftBodyErr != nil {
		return s.draftBodyErr
	}
	s.draftBodies[id] = draft
	return nil
}
func (s *recordingUnifiedStore) UpdateLifecycleStatus(_ context.Context, id, status string) error {
	s.lifecycleSeq[id] = append(s.lifecycleSeq[id], status)
	return nil
}
func (s *recordingUnifiedStore) UpdateSandboxResult(_ context.Context, id string, passed bool, result json.RawMessage) error {
	if s.sandboxWriteErr != nil {
		return s.sandboxWriteErr
	}
	s.sandboxPassed[id] = passed
	s.sandboxResults[id] = result
	return nil
}
func (s *recordingUnifiedStore) ExpireOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func (s *recordingUnifiedStore) UpdateMetadataKey(_ context.Context, id, key, value string) error {
	if s.metaUpdates == nil {
		s.metaUpdates = map[string]map[string]string{}
	}
	if s.metaUpdates[id] == nil {
		s.metaUpdates[id] = map[string]string{}
	}
	s.metaUpdates[id][key] = value
	return nil
}

// stubGateVerifier implements SkillGateVerifier.
type stubGateVerifier struct {
	passed bool
	checks []GateCheckResult
	err    error
	calls  int
}

func (g *stubGateVerifier) Verify(_ context.Context, _, _ string, _ *EvolutionObservationReport) (*GateVerificationResult, error) {
	g.calls++
	if g.err != nil {
		return nil, g.err
	}
	return &GateVerificationResult{Passed: g.passed, Checks: g.checks}, nil
}

func newValidateTestUsecase(store *recordingUnifiedStore, gate SkillGateVerifier) *SkillIntelligenceUsecase {
	lg := loggateway.NewNoop()
	uc := NewSkillIntelligenceUsecase(nil, nil, store, nil, lg)
	if gate != nil {
		uc.SetGate(gate)
	}
	return uc
}

func TestValidatePendingSuggestionsForSkill_NilUnifiedStore(t *testing.T) {
	uc := NewSkillIntelligenceUsecase(nil, nil, nil, nil, loggateway.NewNoop())
	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Errorf("nil unifiedStore should be a no-op, got %v", err)
	}
}

func TestValidatePendingSuggestionsForSkill_EmptySkillID(t *testing.T) {
	uc := newValidateTestUsecase(newRecordingUnifiedStore(), nil)
	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty skill_id")
	}
}

func TestValidatePendingSuggestionsForSkill_ListError(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.listErr = errors.New("db down")
	uc := newValidateTestUsecase(store, nil)
	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err == nil {
		t.Fatal("expected wrapped list error")
	}
}

func TestValidatePendingSuggestionsForSkill_NoPending(t *testing.T) {
	store := newRecordingUnifiedStore()
	uc := newValidateTestUsecase(store, nil)
	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.draftBodies) != 0 {
		t.Error("no pending suggestions — no drafts should be written")
	}
}

func TestValidatePendingSuggestionsForSkill_ValidatesDraftSuggestion(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.pending = []UnifiedEvolutionSuggestion{
		{
			ID:              "sg-1",
			TargetType:      EvolutionTargetSkill,
			TargetID:        "sk-1",
			Status:          "pending",
			LifecycleStatus: "draft",
			DraftBody:       "",
			TriggerReason:   "success rate low",
		},
	}
	uc := newValidateTestUsecase(store, nil)

	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	draft, ok := store.draftBodies["sg-1"]
	if !ok || draft == "" {
		t.Fatal("expected draft body to be generated and persisted")
	}
	seq := store.lifecycleSeq["sg-1"]
	if len(seq) != 2 || seq[0] != "validating" || seq[1] != "draft" {
		t.Errorf("expected lifecycle sequence [validating draft], got %v", seq)
	}
	if _, ok := store.sandboxPassed["sg-1"]; !ok {
		t.Error("expected sandbox result to be recorded")
	}
}

func TestValidatePendingSuggestionsForSkill_SkipsAlreadyValidated(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.pending = []UnifiedEvolutionSuggestion{
		{
			ID:              "sg-validating",
			TargetType:      EvolutionTargetSkill,
			TargetID:        "sk-1",
			Status:          "pending",
			LifecycleStatus: "validating", // mid-validation — skip
		},
		{
			ID:              "sg-has-draft",
			TargetType:      EvolutionTargetSkill,
			TargetID:        "sk-1",
			Status:          "pending",
			LifecycleStatus: "draft",
			DraftBody:       "# existing draft", // already has draft — skip (idempotent)
		},
	}
	uc := newValidateTestUsecase(store, nil)

	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.draftBodies) != 0 {
		t.Errorf("both suggestions should be skipped, got %d draft writes", len(store.draftBodies))
	}
	if len(store.lifecycleSeq) != 0 {
		t.Errorf("no lifecycle transitions expected, got %v", store.lifecycleSeq)
	}
}

func TestValidatePendingSuggestionsForSkill_DraftWriteErrorContinues(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.pending = []UnifiedEvolutionSuggestion{
		{ID: "sg-1", TargetType: EvolutionTargetSkill, TargetID: "sk-1", Status: "pending", LifecycleStatus: "draft"},
	}
	store.draftBodyErr = errors.New("write failed")
	uc := newValidateTestUsecase(store, nil)

	// Validation failure for a suggestion is logged but does not fail the call.
	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("validation errors should not fail the batch, got %v", err)
	}
}

func TestValidatePendingSuggestionsForSkill_UsesGateResult(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.pending = []UnifiedEvolutionSuggestion{
		{ID: "sg-1", TargetType: EvolutionTargetSkill, TargetID: "sk-1", Status: "pending", LifecycleStatus: "draft"},
	}
	gate := &stubGateVerifier{
		passed: false,
		checks: []GateCheckResult{{Name: "functional", Passed: false, Reason: "sandbox failed"}},
	}
	uc := newValidateTestUsecase(store, gate)

	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.calls != 1 {
		t.Errorf("expected gate Verify called once, got %d", gate.calls)
	}
	if store.sandboxPassed["sg-1"] {
		t.Error("sandbox result should reflect gate verdict (passed=false)")
	}
	var payload map[string]any
	if err := json.Unmarshal(store.sandboxResults["sg-1"], &payload); err != nil {
		t.Fatalf("sandbox result should be valid JSON: %v", err)
	}
	if passed, _ := payload["passed"].(bool); passed {
		t.Error("sandbox result payload should record passed=false")
	}
}

func TestValidatePendingSuggestionsForSkill_GateErrorDegradesToFail(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.pending = []UnifiedEvolutionSuggestion{
		{ID: "sg-1", TargetType: EvolutionTargetSkill, TargetID: "sk-1", Status: "pending", LifecycleStatus: "draft"},
	}
	gate := &stubGateVerifier{err: errors.New("sandbox unavailable")}
	uc := newValidateTestUsecase(store, gate)

	if err := uc.ValidatePendingSuggestionsForSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.calls != 1 {
		t.Errorf("expected gate Verify called once, got %d", gate.calls)
	}
	if store.sandboxPassed["sg-1"] {
		t.Error("gate error should degrade to passed=false")
	}
	// Lifecycle must still advance deterministically even on gate failure.
	seq := store.lifecycleSeq["sg-1"]
	if len(seq) != 2 || seq[0] != "validating" || seq[1] != "draft" {
		t.Errorf("expected lifecycle sequence [validating draft] despite gate error, got %v", seq)
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
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, &mockEvolutionStoreBridge{}, agg, lg)

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
	if suggestion.LifecycleStatus != EvoLifecycleDraft {
		t.Errorf("expected lifecycle_status=draft (rule-based template needs human editing), got %q", suggestion.LifecycleStatus)
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
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, &mockEvolutionStoreBridge{}, agg, lg)

	suggestion, err := uc.RunCuratorFlow(context.Background(), "skill-healthy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suggestion != nil {
		t.Errorf("expected nil suggestion for healthy skill, got type=%q", suggestion.Type)
	}
}
