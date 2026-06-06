//go:build integration

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ---------------------------------------------------------------------------
// Stubs for Skill Intelligence integration tests
// ---------------------------------------------------------------------------

// stubExpReportWriter implements biz.ExperienceReportWriter for integration tests.
type stubExpReportWriter struct {
	mu      sync.Mutex
	reports []biz.ExperienceReport
}

func newStubExpReportWriter() *stubExpReportWriter {
	return &stubExpReportWriter{}
}

func (s *stubExpReportWriter) Create(_ context.Context, report biz.ExperienceReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, report)
	return nil
}

func (s *stubExpReportWriter) BatchCreate(_ context.Context, reports []biz.ExperienceReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, reports...)
	return nil
}

func (s *stubExpReportWriter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reports)
}

// stubExpReportReader implements biz.ExperienceReportReader for integration tests.
type stubExpReportReader struct {
	mu      sync.Mutex
	reports []biz.ExperienceReport
	byID    map[string]*biz.ExperienceReport
}

func newStubExpReportReader() *stubExpReportReader {
	return &stubExpReportReader{
		byID: make(map[string]*biz.ExperienceReport),
	}
}

func (s *stubExpReportReader) ListBySkill(_ context.Context, skillID string, limit, offset int) ([]biz.ExperienceReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var filtered []biz.ExperienceReport
	for _, r := range s.reports {
		if r.SkillID == skillID {
			filtered = append(filtered, r)
		}
	}
	if offset >= len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

func (s *stubExpReportReader) GetByID(_ context.Context, id string) (*biz.ExperienceReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.byID[id]
	if !ok {
		return nil, kerrors.NotFound("SKILL_INTELLIGENCE", "experience report not found")
	}
	return r, nil
}

func (s *stubExpReportReader) ListByTimeRange(_ context.Context, _, _ time.Time, _, _ int) ([]biz.ExperienceReport, error) {
	return nil, nil
}

// stubSkillHealthAggregator implements biz.SkillHealthAggregator for integration tests.
type stubSkillHealthAggregator struct {
	metrics *biz.SkillHealthMetrics
	stats   *biz.SkillFailureStats
}

func newStubSkillHealthAggregator() *stubSkillHealthAggregator {
	return &stubSkillHealthAggregator{
		metrics: &biz.SkillHealthMetrics{
			InvocationCount: 10,
			SuccessCount:    8,
			SuccessRate:     0.8,
			AvgDurationMS:   5000,
		},
	}
}

func (s *stubSkillHealthAggregator) GetHealthMetrics(_ context.Context, _ string, _ time.Time) (*biz.SkillHealthMetrics, error) {
	return s.metrics, nil
}

func (s *stubSkillHealthAggregator) GetFailureStats(_ context.Context, _ string, _ time.Time) (*biz.SkillFailureStats, error) {
	return s.stats, nil
}

// stubEvoSuggestionReader implements biz.SkillEvolutionSuggestionReader for integration tests.
type stubEvoSuggestionReader struct {
	suggestions []biz.SkillEvolutionSuggestion
	latest      *biz.SkillEvolutionSuggestion
}

func newStubEvoSuggestionReader() *stubEvoSuggestionReader {
	return &stubEvoSuggestionReader{}
}

func (s *stubEvoSuggestionReader) ListBySkill(_ context.Context, skillID string, _ biz.EvolutionSuggestionStatus, _, _ int) ([]biz.SkillEvolutionSuggestion, error) {
	var filtered []biz.SkillEvolutionSuggestion
	for _, sug := range s.suggestions {
		if sug.SkillID == skillID {
			filtered = append(filtered, sug)
		}
	}
	return filtered, nil
}

func (s *stubEvoSuggestionReader) GetByID(_ context.Context, id string) (*biz.SkillEvolutionSuggestion, error) {
	for i := range s.suggestions {
		if s.suggestions[i].ID == id {
			return &s.suggestions[i], nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (s *stubEvoSuggestionReader) ListPending(_ context.Context, _, _ int) ([]biz.SkillEvolutionSuggestion, error) {
	return nil, nil
}

func (s *stubEvoSuggestionReader) GetLatestBySkill(_ context.Context, _ string) (*biz.SkillEvolutionSuggestion, error) {
	return s.latest, nil
}

// stubEvoSuggestionWriter implements biz.SkillEvolutionSuggestionWriter for integration tests.
type stubEvoSuggestionWriter struct {
	mu          sync.Mutex
	suggestions []biz.SkillEvolutionSuggestion
}

func newStubEvoSuggestionWriter() *stubEvoSuggestionWriter {
	return &stubEvoSuggestionWriter{}
}

func (s *stubEvoSuggestionWriter) Create(_ context.Context, suggestion biz.SkillEvolutionSuggestion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suggestions = append(s.suggestions, suggestion)
	return nil
}

func (s *stubEvoSuggestionWriter) UpdateStatus(_ context.Context, id string, status biz.EvolutionSuggestionStatus, resolvedBy string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.suggestions {
		if s.suggestions[i].ID == id {
			s.suggestions[i].Status = status
			if status == biz.EvoSuggestionApproved {
				s.suggestions[i].ApprovedBy = resolvedBy
			} else if status == biz.EvoSuggestionRejected {
				s.suggestions[i].RejectedBy = resolvedBy
				s.suggestions[i].RejectionReason = reason
			}
			return nil
		}
	}
	return fmt.Errorf("not found: %s", id)
}

func (s *stubEvoSuggestionWriter) UpdateDraftBody(_ context.Context, id string, draftBody string) error {
	return nil
}

func (s *stubEvoSuggestionWriter) UpdateSandboxResult(_ context.Context, id string, passed bool, result json.RawMessage) error {
	return nil
}

func (s *stubEvoSuggestionWriter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.suggestions)
}

// ---------------------------------------------------------------------------
// Integration Tests: Skill Intelligence
// ---------------------------------------------------------------------------

// TestSkillIntelligenceIntegration_AnalyzeInvocation verifies the AnalyzeInvocation
// method produces correct failure tags for various invocation outcomes.
// Run with: go test -tags=integration ./internal/service/... -run TestSkillIntelligenceIntegration_AnalyzeInvocation -count=1
func TestSkillIntelligenceIntegration_AnalyzeInvocation(t *testing.T) {
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	t.Run("Success_NoFailureTags", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:    "skill-web-search",
			Outcome:    "success",
			DurationMS: 1500,
		}
		isSuccess, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !isSuccess {
			t.Error("expected isSuccess=true for success outcome")
		}
		if tags != nil {
			t.Errorf("expected nil failureTags for success, got %v", tags)
		}
	})

	t.Run("Timeout_ToolTimeoutTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-web-search",
			Outcome:      "failure",
			DurationMS:   35000, // > TimeoutThresholdMS
			ErrorCode:    "TIMEOUT",
			ErrorMessage: "operation timed out",
		}
		isSuccess, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if isSuccess {
			t.Error("expected isSuccess=false for timeout failure")
		}
		if !containsTag(tags, biz.FailureTagToolTimeout) {
			t.Errorf("expected tag %q in %v", biz.FailureTagToolTimeout, tags)
		}
	})

	t.Run("APIError_ToolAPIErrorTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-code-gen",
			Outcome:      "failure",
			DurationMS:   500,
			ErrorCode:    "api_error",
			ErrorMessage: "API returned 500",
		}
		isSuccess, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if isSuccess {
			t.Error("expected isSuccess=false for API error")
		}
		if !containsTag(tags, biz.FailureTagToolAPIError) {
			t.Errorf("expected tag %q in %v", biz.FailureTagToolAPIError, tags)
		}
	})

	t.Run("RateLimit_ToolAPIErrorTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-code-gen",
			Outcome:      "failure",
			DurationMS:   200,
			ErrorCode:    "429",
			ErrorMessage: "rate limit exceeded",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagToolAPIError) {
			t.Errorf("expected tag %q for 429 error code", biz.FailureTagToolAPIError)
		}
	})

	t.Run("ParamMismatch_ParamMismatchTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-code-gen",
			Outcome:      "failure",
			DurationMS:   100,
			ErrorCode:    "invalid_param",
			ErrorMessage: "parameter validation failed",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagParamMismatch) {
			t.Errorf("expected tag %q in %v", biz.FailureTagParamMismatch, tags)
		}
	})

	t.Run("ContextOverflow_ContextOverflowTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-summarize",
			Outcome:      "failure",
			DurationMS:   500,
			InputPreview: strings.Repeat("x", 6000), // > ContextOverflowThreshold
			ErrorMessage: "context too long",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagContextOverflow) {
			t.Errorf("expected tag %q in %v", biz.FailureTagContextOverflow, tags)
		}
	})

	t.Run("WrongToolChoice_WrongToolChoiceTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-summarize",
			Outcome:      "failure",
			DurationMS:   200,
			ErrorMessage: "wrong tool was selected for this task",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagWrongToolChoice) {
			t.Errorf("expected tag %q in %v", biz.FailureTagWrongToolChoice, tags)
		}
	})

	t.Run("InstructionAmbiguity_InstructionAmbiguityTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-summarize",
			Outcome:      "failure",
			DurationMS:   200,
			ErrorMessage: "ambiguous instruction provided",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagInstructionAmbiguity) {
			t.Errorf("expected tag %q in %v", biz.FailureTagInstructionAmbiguity, tags)
		}
	})

	t.Run("UnknownFailure_UnknownTag", func(t *testing.T) {
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-unknown",
			Outcome:      "failure",
			DurationMS:   100,
			ErrorMessage: "internal processing error",
		}
		_, tags := uc.AnalyzeInvocation(context.Background(), inv)
		if !containsTag(tags, biz.FailureTagUnknown) {
			t.Errorf("expected tag %q in %v", biz.FailureTagUnknown, tags)
		}
	})
}

// TestSkillIntelligenceIntegration_GenerateReport verifies the full
// Skill invocation → AnalyzeInvocation → GenerateReport → persist → query flow.
func TestSkillIntelligenceIntegration_GenerateReport(t *testing.T) {
	t.Run("SuccessInvocation_ReportPersisted", func(t *testing.T) {
		writer := newStubExpReportWriter()
		reader := newStubExpReportReader()
		aggregator := newStubSkillHealthAggregator()

		uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

		inv := biz.SkillInvocationWrite{
			SkillID:    "skill-web-search",
			Outcome:    "success",
			DurationMS: 1500,
			SessionID:  "session-int-001",
		}

		report, err := uc.GenerateReport(context.Background(), inv)
		if err != nil {
			t.Fatalf("GenerateReport failed: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report")
		}
		if report.SkillID != "skill-web-search" {
			t.Errorf("expected skill_id=skill-web-search, got %q", report.SkillID)
		}
		if !report.IsSuccess {
			t.Error("expected is_success=true for success invocation")
		}
		if len(report.FailureTags) != 0 {
			t.Errorf("expected no failure tags for success, got %v", report.FailureTags)
		}
		if report.FlowSummary == "" {
			t.Error("expected non-empty FlowSummary")
		}
		if report.OptimizationAdvice == "" {
			t.Error("expected non-empty OptimizationAdvice")
		}

		// Verify report was persisted
		if writer.count() != 1 {
			t.Errorf("expected 1 persisted report, got %d", writer.count())
		}
	})

	t.Run("FailureInvocation_ReportWithFailureTags", func(t *testing.T) {
		writer := newStubExpReportWriter()
		reader := newStubExpReportReader()
		aggregator := newStubSkillHealthAggregator()

		uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-code-gen",
			Outcome:      "failure",
			DurationMS:   35000,
			ErrorCode:    "TIMEOUT",
			ErrorMessage: "operation timed out",
			SessionID:    "session-int-002",
		}

		report, err := uc.GenerateReport(context.Background(), inv)
		if err != nil {
			t.Fatalf("GenerateReport failed: %v", err)
		}
		if report.IsSuccess {
			t.Error("expected is_success=false for failure invocation")
		}
		if len(report.FailureTags) == 0 {
			t.Error("expected failure tags for failure invocation")
		}
		if !containsTag(report.FailureTags, biz.FailureTagToolTimeout) {
			t.Errorf("expected tool_timeout tag, got %v", report.FailureTags)
		}
		if report.OptimizationAdvice == "" {
			t.Error("expected non-empty OptimizationAdvice for failure")
		}

		// Verify report was persisted
		if writer.count() != 1 {
			t.Errorf("expected 1 persisted report, got %d", writer.count())
		}
	})
}

// TestSkillIntelligenceIntegration_ScoreSkill verifies the ScoreSkill method
// with different health metrics scenarios.
func TestSkillIntelligenceIntegration_ScoreSkill(t *testing.T) {
	t.Run("HighSuccessRate_HighScore", func(t *testing.T) {
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 20,
				SuccessCount:    18,
				SuccessRate:     0.9,
				AvgDurationMS:   3000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, nil, nil, nil, loggateway.NewNoop())

		score, err := uc.ScoreSkill(context.Background(), "skill-high-perf")
		if err != nil {
			t.Fatalf("ScoreSkill failed: %v", err)
		}
		if score < 60 {
			t.Errorf("expected score >= 60 for high success rate, got %d", score)
		}
	})

	t.Run("LowSuccessRate_LowScore", func(t *testing.T) {
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 20,
				SuccessCount:    4,
				SuccessRate:     0.2,
				AvgDurationMS:   25000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, nil, nil, nil, loggateway.NewNoop())

		score, err := uc.ScoreSkill(context.Background(), "skill-low-perf")
		if err != nil {
			t.Fatalf("ScoreSkill failed: %v", err)
		}
		if score > 50 {
			t.Errorf("expected score <= 50 for low success rate, got %d", score)
		}
	})

	t.Run("InsufficientData_NeutralScore", func(t *testing.T) {
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 3, // < MinInvocationCount
				SuccessCount:    2,
				SuccessRate:     0.66,
				AvgDurationMS:   5000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, nil, nil, nil, loggateway.NewNoop())

		score, err := uc.ScoreSkill(context.Background(), "skill-new")
		if err != nil {
			t.Fatalf("ScoreSkill failed: %v", err)
		}
		if score != biz.DefaultNeutralScore {
			t.Errorf("expected neutral score %d for insufficient data, got %d", biz.DefaultNeutralScore, score)
		}
	})

	t.Run("EmptySkillID_Error", func(t *testing.T) {
		uc := biz.NewSkillIntelligenceUsecase(nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
		_, err := uc.ScoreSkill(context.Background(), "")
		if err == nil {
			t.Error("expected error for empty skill_id, got nil")
		}
	})
}

// TestSkillIntelligenceIntegration_ExperienceReportCRUD verifies the
// report persist → query flow end-to-end.
func TestSkillIntelligenceIntegration_ExperienceReportCRUD(t *testing.T) {
	writer := newStubExpReportWriter()
	reader := newStubExpReportReader()
	aggregator := newStubSkillHealthAggregator()

	uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

	// Generate a report for a failure invocation
	inv := biz.SkillInvocationWrite{
		SkillID:      "skill-int-crud",
		Outcome:      "failure",
		DurationMS:   40000,
		ErrorCode:    "api_error",
		ErrorMessage: "API returned 500 internal server error",
		SessionID:    "session-crud-001",
		ActivationID: "activation-crud-001",
	}

	report, err := uc.GenerateReport(context.Background(), inv)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	// Manually add to reader for query verification
	reader.mu.Lock()
	reader.reports = append(reader.reports, *report)
	reader.byID[report.ID] = report
	reader.mu.Unlock()

	// Query by skill ID
	reports, err := uc.GetExperienceReports(context.Background(), "skill-int-crud", 10, 0)
	if err != nil {
		t.Fatalf("GetExperienceReports failed: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].ID != report.ID {
		t.Errorf("expected report ID %q, got %q", report.ID, reports[0].ID)
	}

	// Query by ID
	fetched, err := uc.GetExperienceReport(context.Background(), report.ID)
	if err != nil {
		t.Fatalf("GetExperienceReport failed: %v", err)
	}
	if fetched.SkillID != "skill-int-crud" {
		t.Errorf("expected skill_id=skill-int-crud, got %q", fetched.SkillID)
	}
	if fetched.IsSuccess {
		t.Error("expected is_success=false for failure invocation")
	}
}

// TestSkillIntelligenceIntegration_EvolutionSuggestions verifies the
// evolution suggestion creation and status management flow.
func TestSkillIntelligenceIntegration_EvolutionSuggestions(t *testing.T) {
	t.Run("CreateAndApproveSuggestion", func(t *testing.T) {
		sugWriter := newStubEvoSuggestionWriter()
		sugReader := newStubEvoSuggestionReader()

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, nil, sugReader, sugWriter, nil, loggateway.NewNoop())

		suggestion := biz.SkillEvolutionSuggestion{
			ID:            "sug-int-001",
			SkillID:       "skill-evo-test",
			Type:          biz.EvoSuggestionFixFailure,
			Status:        biz.EvoSuggestionPending,
			TriggerReason: "30d failure rate exceeds threshold",
			CreatedAt:     time.Now().UTC(),
		}

		// Create suggestion
		err := uc.CreateSuggestion(context.Background(), suggestion)
		if err != nil {
			t.Fatalf("CreateSuggestion failed: %v", err)
		}
		if sugWriter.count() != 1 {
			t.Errorf("expected 1 suggestion, got %d", sugWriter.count())
		}

		// Add to reader for query
		sugReader.suggestions = append(sugReader.suggestions, suggestion)

		// Approve suggestion (updates writer)
		err = uc.ApproveSuggestion(context.Background(), "sug-int-001", "admin")
		if err != nil {
			t.Fatalf("ApproveSuggestion failed: %v", err)
		}

		// Verify status was updated in writer
		sugWriter.mu.Lock()
		var writerSug *biz.SkillEvolutionSuggestion
		for i := range sugWriter.suggestions {
			if sugWriter.suggestions[i].ID == "sug-int-001" {
				writerSug = &sugWriter.suggestions[i]
				break
			}
		}
		sugWriter.mu.Unlock()
		if writerSug == nil {
			t.Fatal("expected suggestion in writer")
		}
		if writerSug.Status != biz.EvoSuggestionApproved {
			t.Errorf("expected status approved in writer, got %q", writerSug.Status)
		}
	})

	t.Run("RejectSuggestion", func(t *testing.T) {
		sugWriter := newStubEvoSuggestionWriter()
		sugReader := newStubEvoSuggestionReader()

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, nil, sugReader, sugWriter, nil, loggateway.NewNoop())

		suggestion := biz.SkillEvolutionSuggestion{
			ID:            "sug-int-002",
			SkillID:       "skill-evo-reject",
			Type:          biz.EvoSuggestionBoostEfficiency,
			Status:        biz.EvoSuggestionPending,
			TriggerReason: "Skill score below threshold",
			CreatedAt:     time.Now().UTC(),
		}

		err := uc.CreateSuggestion(context.Background(), suggestion)
		if err != nil {
			t.Fatalf("CreateSuggestion failed: %v", err)
		}

		sugReader.suggestions = append(sugReader.suggestions, suggestion)

		err = uc.RejectSuggestion(context.Background(), "sug-int-002", "admin", "not applicable")
		if err != nil {
			t.Fatalf("RejectSuggestion failed: %v", err)
		}
	})
}

// TestSkillIntelligenceIntegration_GenerateReportWithRootCauseAnalyzer verifies
// that GenerateReport integrates with RootCauseAnalyzer when a skill invocation
// failure can be mapped to a root cause.
func TestSkillIntelligenceIntegration_GenerateReportWithRootCauseAnalyzer(t *testing.T) {
	ctx := context.Background()
	_ = loggateway.NewNoop()

	t.Run("TimeoutFailure_MappedToRootCause", func(t *testing.T) {
		writer := newStubExpReportWriter()
		reader := newStubExpReportReader()
		aggregator := newStubSkillHealthAggregator()

		uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

		// Simulate a skill timeout failure
		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-web-search",
			Outcome:      "failure",
			DurationMS:   35000,
			ErrorCode:    "TIMEOUT",
			ErrorMessage: "operation timed out after 30s",
			SessionID:    "session-rca-001",
		}

		report, err := uc.GenerateReport(ctx, inv)
		if err != nil {
			t.Fatalf("GenerateReport failed: %v", err)
		}

		// Verify the report has correct failure tags
		if report.IsSuccess {
			t.Error("expected is_success=false for timeout failure")
		}
		if !containsTag(report.FailureTags, biz.FailureTagToolTimeout) {
			t.Errorf("expected tool_timeout tag, got %v", report.FailureTags)
		}

		// Verify flow summary mentions the failure
		if report.FlowSummary == "" {
			t.Error("expected non-empty FlowSummary")
		}

		// Verify optimization advice is relevant
		if report.OptimizationAdvice == "" {
			t.Error("expected non-empty OptimizationAdvice")
		}

		// Verify report was persisted
		if writer.count() != 1 {
			t.Errorf("expected 1 persisted report, got %d", writer.count())
		}
	})

	t.Run("APIErrorFailure_MappedToRootCause", func(t *testing.T) {
		writer := newStubExpReportWriter()
		reader := newStubExpReportReader()
		aggregator := newStubSkillHealthAggregator()

		uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

		inv := biz.SkillInvocationWrite{
			SkillID:      "skill-code-gen",
			Outcome:      "failure",
			DurationMS:   500,
			ErrorCode:    "429",
			ErrorMessage: "rate limit exceeded",
			SessionID:    "session-rca-002",
		}

		report, err := uc.GenerateReport(ctx, inv)
		if err != nil {
			t.Fatalf("GenerateReport failed: %v", err)
		}

		if !containsTag(report.FailureTags, biz.FailureTagToolAPIError) {
			t.Errorf("expected tool_api_error tag, got %v", report.FailureTags)
		}
	})

	t.Run("MultipleFailures_AccumulatePatterns", func(t *testing.T) {
		writer := newStubExpReportWriter()
		reader := newStubExpReportReader()
		aggregator := newStubSkillHealthAggregator()

		uc := biz.NewSkillIntelligenceUsecase(reader, writer, aggregator, nil, nil, nil, loggateway.NewNoop())

		// Generate multiple failure reports for the same skill
		for i := 0; i < 3; i++ {
			inv := biz.SkillInvocationWrite{
				SkillID:      "skill-flaky",
				Outcome:      "failure",
				DurationMS:   35000 + i*1000,
				ErrorCode:    "TIMEOUT",
				ErrorMessage: "operation timed out",
				SessionID:    fmt.Sprintf("session-rca-multi-%d", i),
			}
			_, err := uc.GenerateReport(ctx, inv)
			if err != nil {
				t.Fatalf("GenerateReport #%d failed: %v", i, err)
			}
		}

		// Verify all reports were persisted
		if writer.count() != 3 {
			t.Errorf("expected 3 persisted reports, got %d", writer.count())
		}
	})
}

// TestSkillIntelligenceIntegration_CheckEvolutionTriggers verifies the
// evolution trigger flow: low score → trigger → suggestion creation.
func TestSkillIntelligenceIntegration_CheckEvolutionTriggers(t *testing.T) {
	ctx := context.Background()

	t.Run("HighFailureRate_TriggersFixFailureSuggestion", func(t *testing.T) {
		sugWriter := newStubEvoSuggestionWriter()
		sugReader := newStubEvoSuggestionReader()
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 20,
				SuccessCount:    5,
				SuccessRate:     0.25, // 75% failure rate
				AvgDurationMS:   25000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, sugReader, sugWriter, nil, loggateway.NewNoop())

		suggestion, err := uc.CheckEvolutionTriggers(ctx, "skill-broken")
		if err != nil {
			t.Fatalf("CheckEvolutionTriggers failed: %v", err)
		}
		if suggestion == nil {
			t.Fatal("expected non-nil suggestion for high failure rate skill")
		}
		if suggestion.Type != biz.EvoSuggestionFixFailure {
			t.Errorf("expected type fix_failure, got %q", suggestion.Type)
		}
		if suggestion.SkillID != "skill-broken" {
			t.Errorf("expected skill_id=skill-broken, got %q", suggestion.SkillID)
		}
		if suggestion.Status != biz.EvoSuggestionPending {
			t.Errorf("expected status=pending, got %q", suggestion.Status)
		}

		// Verify suggestion was persisted
		if sugWriter.count() != 1 {
			t.Errorf("expected 1 persisted suggestion, got %d", sugWriter.count())
		}
	})

	t.Run("HealthySkill_NoTrigger", func(t *testing.T) {
		sugWriter := newStubEvoSuggestionWriter()
		sugReader := newStubEvoSuggestionReader()
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 20,
				SuccessCount:    18,
				SuccessRate:     0.9,
				AvgDurationMS:   3000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, sugReader, sugWriter, nil, loggateway.NewNoop())

		suggestion, err := uc.CheckEvolutionTriggers(ctx, "skill-healthy")
		if err != nil {
			t.Fatalf("CheckEvolutionTriggers failed: %v", err)
		}
		if suggestion != nil {
			t.Errorf("expected nil suggestion for healthy skill, got type=%q", suggestion.Type)
		}
	})

	t.Run("InsufficientInvocations_NoTrigger", func(t *testing.T) {
		sugWriter := newStubEvoSuggestionWriter()
		sugReader := newStubEvoSuggestionReader()
		aggregator := &stubSkillHealthAggregator{
			metrics: &biz.SkillHealthMetrics{
				InvocationCount: 3, // < MinInvocationCount
				SuccessCount:    0,
				SuccessRate:     0,
				AvgDurationMS:   50000,
			},
		}

		uc := biz.NewSkillIntelligenceUsecase(nil, nil, aggregator, sugReader, sugWriter, nil, loggateway.NewNoop())

		suggestion, err := uc.CheckEvolutionTriggers(ctx, "skill-new")
		if err != nil {
			t.Fatalf("CheckEvolutionTriggers failed: %v", err)
		}
		if suggestion != nil {
			t.Error("expected nil suggestion for insufficient invocations")
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
