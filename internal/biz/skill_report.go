package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// SkillReportUsecase handles experience report generation and scanning.
// Extracted from SkillIntelligenceUsecase to reduce cognitive complexity (AS-COG-01).
type SkillReportUsecase struct {
	writer           ExperienceReportWriter
	reader           ExperienceReportReader
	statsReader      ExperienceReportStatsReader
	scorer           *SkillScoringUsecase
	analyzer         RootCauseAnalyzer
	unanalyzedReader SkillInvocationUnanalyzedReader
	lg               loggateway.Logger
}

// NewSkillReportUsecase constructs a SkillReportUsecase.
func NewSkillReportUsecase(
	reader ExperienceReportReader,
	writer ExperienceReportWriter,
	statsReader ExperienceReportStatsReader,
	scorer *SkillScoringUsecase,
	analyzer RootCauseAnalyzer,
	lg loggateway.Logger,
) *SkillReportUsecase {
	return &SkillReportUsecase{
		writer:      writer,
		reader:      reader,
		statsReader: statsReader,
		scorer:      scorer,
		analyzer:    analyzer,
		lg:          lg,
	}
}

// SetUnanalyzedReader sets the unanalyzed reader after construction.
// Used by Wire when the reader comes from a separate provider.
func (uc *SkillReportUsecase) SetUnanalyzedReader(r SkillInvocationUnanalyzedReader) {
	uc.unanalyzedReader = r
}

// GenerateReport generates an ExperienceReport for a skill invocation.
// It uses rule-based extraction for structured fields and can optionally
// use LLM for natural language summaries (degradable to rule-based only).
func (uc *SkillReportUsecase) GenerateReport(ctx context.Context, inv SkillInvocationWrite) (*ExperienceReport, error) {
	isSuccess, failureTags := uc.scorer.AnalyzeInvocation(ctx, inv)

	// Compute score.
	score := DefaultNeutralScore // default neutral
	if uc.scorer != nil {
		if s, err := uc.scorer.ScoreSkill(ctx, inv.SkillID); err == nil {
			score = s
		}
	}

	// Build selection snapshot.
	var selectionSnapshot json.RawMessage
	if inv.SelectionReason != nil {
		if data, err := json.Marshal(inv.SelectionReason); err == nil {
			selectionSnapshot = data
		} else {
			uc.lg.Warn("marshal selection reason failed", loggateway.Err(err))
		}
	}

	// Rule-based flow summary.
	flowSummary := buildFlowSummary(inv, isSuccess, failureTags)

	// Rule-based optimization advice.
	optimizationAdvice := buildOptimizationAdvice(inv, isSuccess, failureTags)

	report := &ExperienceReport{
		ID:                 uuid.New().String(),
		TenantID:           "",
		SessionID:          inv.SessionID,
		InvocationID:       inv.ActivationID,
		SkillID:            inv.SkillID,
		SkillName:          inv.SkillName,
		IsSuccess:          isSuccess,
		Score:              score,
		FailureTags:        failureTags,
		FlowSummary:        flowSummary,
		OptimizationAdvice: optimizationAdvice,
		SelectionSnapshot:  selectionSnapshot,
		CreatedAt:          time.Now().UTC(),
	}

	// Root cause analysis for failed invocations.
	if !isSuccess && uc.analyzer != nil {
		if rcaResult, rcaErr := uc.analyzer.AnalyzeInvocationFailure(ctx, inv); rcaErr != nil {
			uc.lg.Warn("GenerateReport: root cause analysis failed",
				loggateway.StepID("skill_intelligence.generate"),
				loggateway.Str("skill_id", inv.SkillID),
				loggateway.Err(rcaErr))
		} else if rcaResult != nil {
			report.RootCauseAnalysis = rcaResult.RootCause
			report.SuggestedFix = rcaResult.FixSuggest
		}
	}

	// Persist the report.
	if uc.writer != nil {
		if err := uc.writer.Create(ctx, *report); err != nil {
			uc.lg.Warn("GenerateReport: write failed",
				loggateway.StepID("skill_intelligence.generate"),
				loggateway.Str("skill_id", inv.SkillID),
				loggateway.Err(err))
			return nil, fmt.Errorf("persist experience report: %w", err)
		}
	}

	return report, nil
}

// GetExperienceReports lists experience reports for a skill.
func (uc *SkillReportUsecase) GetExperienceReports(ctx context.Context, skillID string, limit, offset int) ([]ExperienceReport, error) {
	if uc.reader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.ListBySkill(ctx, skillID, limit, offset)
}

// GetExperienceReportsFiltered returns experience reports with optional skillID
// and time range filters, along with total count for pagination and aggregate
// statistics (failure tag counts and root cause reports).
// skillID empty = no skill filter; startTime/endTime nil = no time boundary.
func (uc *SkillReportUsecase) GetExperienceReportsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit, offset int) (*ExperienceReportListResult, error) {
	if uc.reader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}

	reports, totalCount, err := uc.reader.ListFiltered(ctx, skillID, startTime, endTime, limit, offset)
	if err != nil {
		uc.lg.Warn("GetExperienceReportsFiltered: ListFiltered failed",
			loggateway.StepID("skill_intelligence.list_filtered"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}

	result := &ExperienceReportListResult{
		Reports:    reports,
		TotalCount: totalCount,
	}

	// Fetch failure tag counts if stats reader is available.
	if uc.statsReader != nil {
		tagCounts, tagErr := uc.statsReader.GetFailureTagCountsFiltered(ctx, skillID, startTime, endTime)
		if tagErr != nil {
			uc.lg.Warn("GetExperienceReportsFiltered: GetFailureTagCountsFiltered failed",
				loggateway.StepID("skill_intelligence.list_filtered"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(tagErr))
		} else {
			result.FailureTagCounts = tagCounts
		}

		rootCauseReports, rcErr := uc.statsReader.GetRootCauseReportsFiltered(ctx, skillID, startTime, endTime, 10)
		if rcErr != nil {
			uc.lg.Warn("GetExperienceReportsFiltered: GetRootCauseReportsFiltered failed",
				loggateway.StepID("skill_intelligence.list_filtered"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(rcErr))
		} else {
			result.RootCauseReports = rootCauseReports
		}
	}

	return result, nil
}

// GetExperienceReport returns a single experience report by ID.
func (uc *SkillReportUsecase) GetExperienceReport(ctx context.Context, id string) (*ExperienceReport, error) {
	if uc.reader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.GetByID(ctx, id)
}

// ScanAndGenerateReports scans recent skill invocations that don't have
// experience reports yet and generates reports for them.
func (uc *SkillReportUsecase) ScanAndGenerateReports(ctx context.Context) error {
	if uc.unanalyzedReader == nil {
		uc.lg.Info("SkillIntelligenceWorker: no unanalyzed reader configured, skipping scan",
			loggateway.StepID("skill_intelligence.scan"))
		return nil
	}

	const batchSize = 100
	invs, err := uc.unanalyzedReader.ListUnanalyzed(ctx, batchSize)
	if err != nil {
		uc.lg.Warn("SkillIntelligenceWorker: ListUnanalyzed failed",
			loggateway.StepID("skill_intelligence.scan"),
			loggateway.Err(err))
		return err
	}

	if len(invs) == 0 {
		return nil
	}

	uc.lg.Info("SkillIntelligenceWorker: processing unanalyzed invocations",
		loggateway.StepID("skill_intelligence.scan"),
		loggateway.Int("count", len(invs)))

	for _, inv := range invs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// GenerateReport handles AnalyzeInvocation + ScoreSkill + RCA internally.
		if _, genErr := uc.GenerateReport(ctx, inv); genErr != nil {
			uc.lg.Warn("SkillIntelligenceWorker: GenerateReport failed for invocation",
				loggateway.StepID("skill_intelligence.scan"),
				loggateway.Str("invocation_id", inv.ActivationID),
				loggateway.Err(genErr))
			continue
		}
		// Only mark as analyzed when report generation succeeded.
		if markErr := uc.unanalyzedReader.MarkAnalyzed(ctx, inv.ActivationID); markErr != nil {
			uc.lg.Warn("SkillIntelligenceWorker: MarkAnalyzed failed",
				loggateway.StepID("skill_intelligence.scan"),
				loggateway.Str("invocation_id", inv.ActivationID),
				loggateway.Err(markErr))
		}
	}

	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildFlowSummary(inv SkillInvocationWrite, isSuccess bool, failureTags []string) string {
	if isSuccess {
		return fmt.Sprintf("Skill %s completed successfully in %dms.", inv.SkillID, inv.DurationMS)
	}
	tagsStr := "unknown"
	if len(failureTags) > 0 {
		tagsStr = strings.Join(failureTags, ", ")
	}
	errMsg := inv.ErrorMessage
	if errMsg == "" {
		errMsg = inv.ErrorCode
	}
	if errMsg != "" {
		return fmt.Sprintf("Skill %s failed in %dms. Failure tags: [%s]. Error: %s", inv.SkillID, inv.DurationMS, tagsStr, truncateStr(errMsg, 200))
	}
	return fmt.Sprintf("Skill %s failed in %dms. Failure tags: [%s].", inv.SkillID, inv.DurationMS, tagsStr)
}

func buildOptimizationAdvice(inv SkillInvocationWrite, isSuccess bool, failureTags []string) string {
	if isSuccess {
		if inv.DurationMS > 10000 {
			return "Consider optimizing skill performance to reduce latency."
		}
		return "No optimization needed."
	}

	var advices []string
	for _, tag := range failureTags {
		switch tag {
		case FailureTagToolTimeout:
			advices = append(advices, "Add timeout handling and retry logic to the skill implementation")
		case FailureTagToolAPIError:
			advices = append(advices, "Add error handling for API failures and implement fallback behavior")
		case FailureTagParamMismatch:
			advices = append(advices, "Improve parameter validation and add clearer parameter descriptions")
		case FailureTagWrongToolChoice:
			advices = append(advices, "Refine skill description to reduce ambiguity in tool selection")
		case FailureTagContextOverflow:
			advices = append(advices, "Reduce input size or add context window management")
		case FailureTagInstructionAmbiguity:
			advices = append(advices, "Clarify skill instructions and add examples")
		default:
			advices = append(advices, "Investigate root cause and add error handling")
		}
	}
	if len(advices) == 0 {
		return "Investigate failure and add appropriate error handling."
	}
	return strings.Join(advices, "; ")
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
