package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/types"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/experiencereport"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/loggateway"
)

type SkillIntelligenceRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.SkillIntelligenceRepo           = (*SkillIntelligenceRepo)(nil)
	_ biz.ExperienceReportReader          = (*SkillIntelligenceRepo)(nil)
	_ biz.ExperienceReportStatsReader     = (*SkillIntelligenceRepo)(nil)
	_ biz.ExperienceReportWriter          = (*SkillIntelligenceRepo)(nil)
	_ biz.SkillHealthAggregator           = (*SkillIntelligenceRepo)(nil)
	_ biz.SkillInvocationUnanalyzedReader = (*SkillIntelligenceRepo)(nil)
)

// NewSkillIntelligenceRepo creates a new SkillIntelligenceRepo.
func NewSkillIntelligenceRepo(data *Data, lg loggateway.Logger) *SkillIntelligenceRepo {
	return &SkillIntelligenceRepo{data: data, lg: lg}
}

// ── ExperienceReportReader ────────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) ListBySkill(ctx context.Context, skillID string, limit, offset int) ([]biz.ExperienceReport, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(experiencereport.SkillIDEQ(skillID)).
		Order(ent.Desc(experiencereport.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapEntReports(rows), nil
}

func (r *SkillIntelligenceRepo) GetByID(ctx context.Context, id string) (*biz.ExperienceReport, error) {
	row, err := r.data.RW().Read(ctx).ExperienceReport.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	report := mapEntReport(row)
	return &report, nil
}

func (r *SkillIntelligenceRepo) ListByTimeRange(ctx context.Context, from, to time.Time, limit, offset int) ([]biz.ExperienceReport, error) {
	if limit <= 0 {
		limit = 50
	}
	fromStr := from.UTC().Format(time.RFC3339)
	toStr := to.UTC().Format(time.RFC3339)
	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(
			experiencereport.CreatedAtGTE(fromStr),
			experiencereport.CreatedAtLTE(toStr),
		).
		Order(ent.Desc(experiencereport.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapEntReports(rows), nil
}

// buildExperienceReportPredicates constructs dynamic Ent predicates from optional
// skillID and time range filters. Empty skillID = no skill filter; nil time = no time boundary.
func buildExperienceReportPredicates(skillID string, startTime, endTime *time.Time) []predicate.ExperienceReport {
	var preds []predicate.ExperienceReport
	if skillID != "" {
		preds = append(preds, experiencereport.SkillIDEQ(skillID))
	}
	if startTime != nil {
		preds = append(preds, experiencereport.CreatedAtGTE(startTime.UTC().Format(time.RFC3339)))
	}
	if endTime != nil {
		preds = append(preds, experiencereport.CreatedAtLTE(endTime.UTC().Format(time.RFC3339)))
	}
	return preds
}

// ── ExperienceReportReader (ListFiltered) ─────────────────────────────────────

func (r *SkillIntelligenceRepo) ListFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit, offset int) ([]biz.ExperienceReport, int, error) {
	if limit <= 0 {
		limit = 50
	}
	preds := buildExperienceReportPredicates(skillID, startTime, endTime)

	count, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		Order(ent.Desc(experiencereport.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return mapEntReports(rows), count, nil
}

// ── ExperienceReportStatsReader ────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) GetFailureTagCountsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time) ([]biz.FailureTagCount, error) {
	preds := buildExperienceReportPredicates(skillID, startTime, endTime)
	preds = append(preds, experiencereport.IsSuccessEQ(false))

	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		All(ctx)
	if err != nil {
		return nil, err
	}

	tagCountMap := make(map[string]int)
	for _, row := range rows {
		for _, tag := range row.FailureTags {
			tagCountMap[tag]++
		}
	}

	result := make([]biz.FailureTagCount, 0, len(tagCountMap))
	for tag, count := range tagCountMap {
		result = append(result, biz.FailureTagCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result, nil
}

func (r *SkillIntelligenceRepo) GetRootCauseReportsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit int) ([]biz.ExperienceReport, error) {
	if limit <= 0 {
		limit = 10
	}
	preds := buildExperienceReportPredicates(skillID, startTime, endTime)
	preds = append(preds, experiencereport.IsSuccessEQ(false))
	preds = append(preds, experiencereport.Or(
		experiencereport.RootCauseAnalysisNEQ(""),
		experiencereport.SuggestedFixNEQ(""),
	))

	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		Order(ent.Desc(experiencereport.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapEntReports(rows), nil
}

// ── ExperienceReportWriter ────────────────────────────────────────────────────

// reportToCreateBuilder creates an Ent builder pre-filled with all report fields.
// Caller must call Save(ctx) on the returned builder.
func (r *SkillIntelligenceRepo) reportToCreateBuilder(ctx context.Context, report biz.ExperienceReport) (*ent.ExperienceReportCreate, error) {
	builder := r.data.RW().Write(ctx).ExperienceReport.Create().
		SetID(report.ID).
		SetTenantID(report.TenantID).
		SetSessionID(report.SessionID).
		SetInvocationID(report.InvocationID).
		SetSkillID(report.SkillID).
		SetSkillName(report.SkillName).
		SetIsSuccess(report.IsSuccess).
		SetScore(report.Score).
		SetFlowSummary(report.FlowSummary).
		SetOptimizationAdvice(report.OptimizationAdvice).
		SetRootCauseAnalysis(report.RootCauseAnalysis).
		SetSuggestedFix(report.SuggestedFix).
		SetCreatedAt(report.CreatedAt.UTC().Format(time.RFC3339))
	if len(report.FailureTags) > 0 {
		builder.SetFailureTags(report.FailureTags)
	}
	if report.SelectionSnapshot != nil {
		var snap map[string]any
		if err := json.Unmarshal(report.SelectionSnapshot, &snap); err != nil {
			return nil, fmt.Errorf("invalid selection_snapshot JSON for report %s: %w", report.ID, err)
		}
		builder.SetSelectionSnapshot(snap)
	}
	if report.GeneratedSuggestionID != nil && *report.GeneratedSuggestionID != "" {
		builder.SetGeneratedSuggestionID(*report.GeneratedSuggestionID)
	}
	return builder, nil
}

func (r *SkillIntelligenceRepo) Create(ctx context.Context, report biz.ExperienceReport) error {
	builder, err := r.reportToCreateBuilder(ctx, report)
	if err != nil {
		return err
	}
	_, err = builder.Save(ctx)
	return err
}

func (r *SkillIntelligenceRepo) BatchCreate(ctx context.Context, reports []biz.ExperienceReport) error {
	if len(reports) == 0 {
		return nil
	}
	builders := make([]*ent.ExperienceReportCreate, 0, len(reports))
	for _, report := range reports {
		builder, err := r.reportToCreateBuilder(ctx, report)
		if err != nil {
			return err
		}
		builders = append(builders, builder)
	}
	_, err := r.data.RW().Write(ctx).ExperienceReport.CreateBulk(builders...).Save(ctx)
	return err
}

// ── SkillHealthAggregator ─────────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) GetHealthMetrics(ctx context.Context, skillID string, since time.Time) (*biz.SkillHealthMetrics, error) {
	sinceStr := since.UTC().Format(time.RFC3339)

	// Main aggregation query — computes count, success_count, avg_duration, avg_token_usage in SQL.
	const aggQuery = `SELECT
  COUNT(*) as invocation_count,
  SUM(CASE WHEN outcome = 'success' OR (outcome = '' AND (status = 'completed' OR status = 'success')) THEN 1 ELSE 0 END) as success_count,
  AVG(CASE WHEN duration_ms > 0 THEN duration_ms END) as avg_duration_ms,
  COALESCE(AVG(CASE WHEN token_usage IS NOT NULL THEN json_extract(token_usage, '$.total') END), 0) as avg_token_usage
FROM skill_invocation
WHERE skill_id = ? AND created_at >= ?`

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, aggQuery, skillID, sinceStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metrics := &biz.SkillHealthMetrics{SkillID: skillID}
	if rows.Next() {
		var invocationCount, successCount int
		var avgDurationMS, avgTokenUsage float64
		if err := rows.Scan(&invocationCount, &successCount, &avgDurationMS, &avgTokenUsage); err != nil {
			return nil, err
		}
		metrics.InvocationCount = invocationCount
		metrics.SuccessCount = successCount
		metrics.AvgDurationMS = avgDurationMS
		metrics.AvgTokenUsage = int(avgTokenUsage)
		if invocationCount > 0 {
			metrics.SuccessRate = float64(successCount) / float64(invocationCount)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// P95: for small row counts (<20), load durations and compute in Go;
	// for larger sets, use SQL ORDER BY + OFFSET approach.
	const countQuery = `SELECT COUNT(*) FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0`
	var durCount int
	cRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, countQuery, skillID, sinceStr)
	if err != nil {
		return nil, err
	}
	defer cRows.Close()
	if cRows.Next() {
		if err := cRows.Scan(&durCount); err != nil {
			return nil, err
		}
	}
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	if durCount == 0 {
		metrics.P95DurationMS = 0
	} else if durCount < 20 {
		// Small set: load all durations and compute P95 in Go.
		const durQuery = `SELECT duration_ms FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0 ORDER BY duration_ms ASC`
		dRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, durQuery, skillID, sinceStr)
		if err != nil {
			return nil, err
		}
		defer dRows.Close()
		var durations []int
		for dRows.Next() {
			var d int
			if err := dRows.Scan(&d); err != nil {
				return nil, err
			}
			durations = append(durations, d)
		}
		if err := dRows.Err(); err != nil {
			return nil, err
		}
		metrics.P95DurationMS = types.P95(durations)
	} else {
		// Large set: use SQL OFFSET to pick the 95th percentile row.
		offset := durCount * 95 / 100
		if offset >= durCount {
			offset = durCount - 1
		}
		const p95Query = `SELECT duration_ms FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0 ORDER BY duration_ms ASC LIMIT 1 OFFSET ?`
		var p95 int
		pRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, p95Query, skillID, sinceStr, offset)
		if err != nil {
			return nil, err
		}
		defer pRows.Close()
		if pRows.Next() {
			if err := pRows.Scan(&p95); err != nil {
				return nil, err
			}
		}
		if err := pRows.Err(); err != nil {
			return nil, err
		}
		metrics.P95DurationMS = p95
	}

	// FeedbackScore: not yet available from DB; will be computed heuristically in biz layer.
	return metrics, nil
}

// extractTokenTotal extracts the "total" field from a token_usage JSON map.
func extractTokenTotal(tokenUsage map[string]any) (int, bool) {
	if tokenUsage == nil {
		return 0, false
	}
	total, ok := tokenUsage["total"]
	if !ok {
		return 0, false
	}
	switch v := total.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

func (r *SkillIntelligenceRepo) GetFailureStats(ctx context.Context, skillID string, since time.Time) (*biz.SkillFailureStats, error) {
	sinceStr := since.UTC().Format(time.RFC3339)

	// Count total failures.
	const failCountQuery = `SELECT COUNT(*) FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND outcome != 'success' AND NOT (outcome = '' AND (status = 'completed' OR status = 'success'))`
	var failureCount int
	fRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, failCountQuery, skillID, sinceStr)
	if err != nil {
		return nil, err
	}
	defer fRows.Close()
	if fRows.Next() {
		if err := fRows.Scan(&failureCount); err != nil {
			return nil, err
		}
	}
	if err := fRows.Err(); err != nil {
		return nil, err
	}

	// Top error codes via SQL GROUP BY.
	const topCodesQuery = `SELECT
  COALESCE(error_code, 'unknown') as error_code,
  COUNT(*) as count
FROM skill_invocation
WHERE skill_id = ? AND created_at >= ? AND outcome != 'success' AND NOT (outcome = '' AND (status = 'completed' OR status = 'success'))
GROUP BY error_code
ORDER BY count DESC
LIMIT 5`
	cRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, topCodesQuery, skillID, sinceStr)
	if err != nil {
		return nil, err
	}
	defer cRows.Close()

	var topCodes []biz.ErrorCodeCount
	for cRows.Next() {
		var code string
		var count int
		if err := cRows.Scan(&code, &count); err != nil {
			return nil, err
		}
		topCodes = append(topCodes, biz.ErrorCodeCount{ErrorCode: code, Count: count})
	}
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	return &biz.SkillFailureStats{
		SkillID:       skillID,
		FailureCount:  failureCount,
		TopErrorCodes: topCodes,
	}, nil
}

// GetFailureTagCounts returns failure tag counts for a skill over the given
// time window by querying experience reports.
func (r *SkillIntelligenceRepo) GetFailureTagCounts(ctx context.Context, skillID string, since time.Time) ([]biz.FailureTagCount, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(
			experiencereport.SkillIDEQ(skillID),
			experiencereport.CreatedAtGTE(sinceStr),
			experiencereport.IsSuccessEQ(false),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	tagCountMap := make(map[string]int)
	for _, row := range rows {
		for _, tag := range row.FailureTags {
			tagCountMap[tag]++
		}
	}

	result := make([]biz.FailureTagCount, 0, len(tagCountMap))
	for tag, count := range tagCountMap {
		result = append(result, biz.FailureTagCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result, nil
}

// ── Mapping helpers ───────────────────────────────────────────────────────────

func mapEntReport(row *ent.ExperienceReport) biz.ExperienceReport {
	report := biz.ExperienceReport{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		SessionID:          row.SessionID,
		InvocationID:       row.InvocationID,
		SkillID:            row.SkillID,
		SkillName:          row.SkillName,
		IsSuccess:          row.IsSuccess,
		Score:              row.Score,
		FailureTags:        row.FailureTags,
		FlowSummary:        row.FlowSummary,
		OptimizationAdvice: row.OptimizationAdvice,
		RootCauseAnalysis:  row.RootCauseAnalysis,
		SuggestedFix:       row.SuggestedFix,
	}
	if row.SelectionSnapshot != nil {
		if data, err := json.Marshal(row.SelectionSnapshot); err == nil {
			report.SelectionSnapshot = data
		}
	}
	if row.GeneratedSuggestionID != "" {
		sid := row.GeneratedSuggestionID
		report.GeneratedSuggestionID = &sid
	}
	if row.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, row.CreatedAt); err == nil {
			report.CreatedAt = t
		}
	}
	return report
}

func mapEntReports(rows []*ent.ExperienceReport) []biz.ExperienceReport {
	result := make([]biz.ExperienceReport, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapEntReport(row))
	}
	return result
}

// ── SkillInvocationUnanalyzedReader ───────────────────────────────────────────

func (r *SkillIntelligenceRepo) ListUnanalyzed(ctx context.Context, batchSize int) ([]biz.SkillInvocationWrite, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.AnalyzedAtEQ(""),
			skillinvocation.OutcomeNEQ(""),
		).
		Order(ent.Asc(skillinvocation.FieldCreatedAt)).
		Limit(batchSize).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]biz.SkillInvocationWrite, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapEntSkillInvocationToWrite(row))
	}
	return result, nil
}

func (r *SkillIntelligenceRepo) MarkAnalyzed(ctx context.Context, activationID string) error {
	if activationID == "" {
		return nil
	}
	// Single UPDATE with WHERE activation_id — avoids two-step read-then-write.
	n, err := r.data.RW().Write(ctx).SkillInvocation.Update().
		Where(skillinvocation.ActivationIDEQ(activationID)).
		SetAnalyzedAt(time.Now().UTC().Format(time.RFC3339)).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("skill invocation with activation_id %s not found", activationID)
	}
	return nil
}

func mapEntSkillInvocationToWrite(row *ent.SkillInvocation) biz.SkillInvocationWrite {
	return biz.SkillInvocationWrite{
		SkillID:         row.SkillID,
		SkillVersion:    row.SkillVersion,
		AgentID:         row.AgentID,
		UserID:          row.UserID,
		SessionID:       row.SessionID,
		Status:          row.Status,
		DurationMS:      row.DurationMs,
		StartedAt:       row.StartedAt,
		EndedAt:         row.EndedAt,
		InputPreview:    row.InputPreview,
		InputHash:       row.InputHash,
		OutputPreview:   row.OutputPreview,
		ErrorCode:       row.ErrorCode,
		ErrorMessage:    row.ErrorMessage,
		Source:          row.Source,
		ActivationID:    row.ActivationID,
		MessageID:       row.MessageID,
		SelectionReason: row.SelectionReason,
		Outcome:         row.Outcome,
		TokenUsage:      row.TokenUsage,
		RoutedSlugs:     row.RoutedSlugs,
		LoadedSlug:      row.LoadedSlug,
	}
}
