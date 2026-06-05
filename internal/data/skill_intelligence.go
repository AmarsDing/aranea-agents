package data

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/experiencereport"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/loggateway"
)

type SkillIntelligenceRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.ExperienceReportReader  = (*SkillIntelligenceRepo)(nil)
	_ biz.ExperienceReportWriter  = (*SkillIntelligenceRepo)(nil)
	_ biz.SkillHealthAggregator   = (*SkillIntelligenceRepo)(nil)
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

// ── ExperienceReportWriter ────────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) Create(ctx context.Context, report biz.ExperienceReport) error {
	builder := r.data.RW().Write(ctx).ExperienceReport.Create().
		SetID(report.ID).
		SetTenantID(report.TenantID).
		SetSessionID(report.SessionID).
		SetInvocationID(report.InvocationID).
		SetSkillID(report.SkillID).
		SetIsSuccess(report.IsSuccess).
		SetScore(report.Score).
		SetFlowSummary(report.FlowSummary).
		SetOptimizationAdvice(report.OptimizationAdvice).
		SetCreatedAt(report.CreatedAt.UTC().Format(time.RFC3339))
	if len(report.FailureTags) > 0 {
		builder.SetFailureTags(report.FailureTags)
	}
	if report.SelectionSnapshot != nil {
		var snap map[string]interface{}
		if err := json.Unmarshal(report.SelectionSnapshot, &snap); err == nil {
			builder.SetSelectionSnapshot(snap)
		}
	}
	if report.GeneratedSuggestionID != nil && *report.GeneratedSuggestionID != "" {
		builder.SetGeneratedSuggestionID(*report.GeneratedSuggestionID)
	}
	_, err := builder.Save(ctx)
	return err
}

func (r *SkillIntelligenceRepo) BatchCreate(ctx context.Context, reports []biz.ExperienceReport) error {
	if len(reports) == 0 {
		return nil
	}
	builders := make([]*ent.ExperienceReportCreate, 0, len(reports))
	for _, report := range reports {
		builder := r.data.RW().Write(ctx).ExperienceReport.Create().
			SetID(report.ID).
			SetTenantID(report.TenantID).
			SetSessionID(report.SessionID).
			SetInvocationID(report.InvocationID).
			SetSkillID(report.SkillID).
			SetIsSuccess(report.IsSuccess).
			SetScore(report.Score).
			SetFlowSummary(report.FlowSummary).
			SetOptimizationAdvice(report.OptimizationAdvice).
			SetCreatedAt(report.CreatedAt.UTC().Format(time.RFC3339))
		if len(report.FailureTags) > 0 {
			builder.SetFailureTags(report.FailureTags)
		}
		if report.SelectionSnapshot != nil {
			var snap map[string]interface{}
			if err := json.Unmarshal(report.SelectionSnapshot, &snap); err == nil {
				builder.SetSelectionSnapshot(snap)
			}
		}
		if report.GeneratedSuggestionID != nil {
			builder.SetGeneratedSuggestionID(*report.GeneratedSuggestionID)
		}
		builders = append(builders, builder)
	}
	_, err := r.data.RW().Write(ctx).ExperienceReport.CreateBulk(builders...).Save(ctx)
	return err
}

// ── SkillHealthAggregator ─────────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) GetHealthMetrics(ctx context.Context, skillID string, since time.Time) (*biz.SkillHealthMetrics, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.SkillIDEQ(skillID),
			skillinvocation.CreatedAtGTE(sinceStr),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	metrics := &biz.SkillHealthMetrics{SkillID: skillID}
	var totalDuration int
	var durations []int

	for _, row := range rows {
		metrics.InvocationCount++
		if isSuccess(row.Outcome, row.Status) {
			metrics.SuccessCount++
		}
		totalDuration += row.DurationMs
		durations = append(durations, row.DurationMs)
	}

	if metrics.InvocationCount > 0 {
		metrics.SuccessRate = float64(metrics.SuccessCount) / float64(metrics.InvocationCount)
		metrics.AvgDurationMS = float64(totalDuration) / float64(metrics.InvocationCount)
	}
	metrics.P95DurationMS = p95(durations)
	return metrics, nil
}

func (r *SkillIntelligenceRepo) GetFailureStats(ctx context.Context, skillID string, since time.Time) (*biz.SkillFailureStats, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.SkillIDEQ(skillID),
			skillinvocation.CreatedAtGTE(sinceStr),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	stats := &biz.SkillFailureStats{SkillID: skillID}
	codeCount := make(map[string]int)
	for _, row := range rows {
		if !isSuccess(row.Outcome, row.Status) {
			stats.FailureCount++
			code := row.ErrorCode
			if code == "" {
				code = "unknown"
			}
			codeCount[code]++
		}
	}

	for code, count := range codeCount {
		stats.TopErrorCodes = append(stats.TopErrorCodes, biz.ErrorCodeCount{ErrorCode: code, Count: count})
	}
	sort.Slice(stats.TopErrorCodes, func(i, j int) bool {
		return stats.TopErrorCodes[i].Count > stats.TopErrorCodes[j].Count
	})
	if len(stats.TopErrorCodes) > 5 {
		stats.TopErrorCodes = stats.TopErrorCodes[:5]
	}
	return stats, nil
}

// ── Mapping helpers ───────────────────────────────────────────────────────────

func mapEntReport(row *ent.ExperienceReport) biz.ExperienceReport {
	report := biz.ExperienceReport{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		SessionID:          row.SessionID,
		InvocationID:       row.InvocationID,
		SkillID:            row.SkillID,
		IsSuccess:          row.IsSuccess,
		Score:              row.Score,
		FailureTags:        row.FailureTags,
		FlowSummary:        row.FlowSummary,
		OptimizationAdvice: row.OptimizationAdvice,
	}
	if row.SelectionSnapshot != nil {
		report.SelectionSnapshot, _ = json.Marshal(row.SelectionSnapshot)
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
