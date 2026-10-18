package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/types"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/experiencereport"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type SkillIntelligenceRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
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
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	reports := mapEntReports(rows)
	r.backfillReportSkillNames(ctx, reports)
	return reports, nil
}

func (r *SkillIntelligenceRepo) GetByID(ctx context.Context, id string) (*biz.ExperienceReport, error) {
	row, err := r.data.RW().Read(ctx).ExperienceReport.Get(ctx, id)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	report := mapEntReport(row)
	if report.SkillName == "" && report.SkillID != "" {
		reports := []biz.ExperienceReport{report}
		r.backfillReportSkillNames(ctx, reports)
		report = reports[0]
	}
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
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	reports := mapEntReports(rows)
	r.backfillReportSkillNames(ctx, reports)
	return reports, nil
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
		return nil, 0, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	rows, err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		Order(ent.Desc(experiencereport.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	reports := mapEntReports(rows)
	r.backfillReportSkillNames(ctx, reports)
	return reports, count, nil
}

// ── ExperienceReportStatsReader ────────────────────────────────────────────────

// GetFailureTagCountsFiltered 审计 T8：失败标签聚类从「全行拉取 + Go 内存
// 计数」下沉为方言感知 SQL 数组展开 + GROUP BY——失败行随时间无界增长，
// 内存聚合会持续退化。谓词与 buildExperienceReportPredicates 语义等价；
// NULL/空数组行经 json_each / json_array_elements_text 展开后为零元素，
// 行为与原 Go 循环一致。
func (r *SkillIntelligenceRepo) GetFailureTagCountsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time) ([]biz.FailureTagCount, error) {
	conds := []string{"is_success = false"}
	var args []any
	if skillID != "" {
		conds = append(conds, "skill_id = ?")
		args = append(args, skillID)
	}
	if startTime != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, startTime.UTC().Format(time.RFC3339))
	}
	if endTime != nil {
		conds = append(conds, "created_at <= ?")
		args = append(args, endTime.UTC().Format(time.RFC3339))
	}
	return r.queryFailureTagCounts(ctx, conds, args)
}

// queryFailureTagCounts 执行失败标签 SQL 聚合（T8 两入口共用）：JSON 数组
// 展开（dialect.JSONEach）后按 tag GROUP BY，ORDER BY count desc + tag 升序
// 保证确定性排序。无命中时返回空的非 nil 切片（与原实现一致）。
func (r *SkillIntelligenceRepo) queryFailureTagCounts(ctx context.Context, conds []string, args []any) ([]biz.FailureTagCount, error) {
	d := r.data.Dialect()
	query := fmt.Sprintf(
		`SELECT value AS tag, COUNT(*) AS cnt FROM %s, %s WHERE %s GROUP BY value ORDER BY cnt DESC, value ASC`,
		experiencereport.Table, d.JSONEach(experiencereport.FieldFailureTags), strings.Join(conds, " AND "))
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, d.RenumberPlaceholders(query), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	defer rows.Close()

	result := make([]biz.FailureTagCount, 0, 8)
	for rows.Next() {
		var fc biz.FailureTagCount
		if err := rows.Scan(&fc.Tag, &fc.Count); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		result = append(result, fc)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
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
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	reports := mapEntReports(rows)
	r.backfillReportSkillNames(ctx, reports)
	return reports, nil
}

// GetExperienceReportStatsFiltered aggregates success/failure counts and the overall
// average score in one GROUP BY query. AvgScore 为全量记录的加权平均（ent 的 Mean
// 按 is_success 分组计算，需按组计数加权重算总平均）。
func (r *SkillIntelligenceRepo) GetExperienceReportStatsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time) (*biz.ExperienceReportStats, error) {
	preds := buildExperienceReportPredicates(skillID, startTime, endTime)

	var aggRows []struct {
		IsSuccess bool    `json:"is_success"`
		Count     int     `json:"count"`
		AvgScore  float64 `json:"avg_score"`
	}
	err := r.data.RW().Read(ctx).ExperienceReport.Query().
		Where(preds...).
		GroupBy(experiencereport.FieldIsSuccess).
		Aggregate(
			ent.Count(),
			ent.As(ent.Mean(experiencereport.FieldScore), "avg_score"),
		).
		Scan(ctx, &aggRows)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	stats := &biz.ExperienceReportStats{}
	var weightedSum float64
	for _, row := range aggRows {
		if row.IsSuccess {
			stats.SuccessCount = row.Count
		} else {
			stats.FailureCount = row.Count
		}
		weightedSum += row.AvgScore * float64(row.Count)
	}
	if total := stats.SuccessCount + stats.FailureCount; total > 0 {
		stats.AvgScore = weightedSum / float64(total)
	}
	return stats, nil
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
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		builder.SetSelectionSnapshot(snap)
	}
	if report.GeneratedSuggestionID != nil && *report.GeneratedSuggestionID != "" {
		builder.SetGeneratedSuggestionID(*report.GeneratedSuggestionID)
	}
	return builder, nil
}

func (r *SkillIntelligenceRepo) Create(ctx context.Context, report biz.ExperienceReport) error {
	// 审计 W5① 幂等：invocation_id 仅普通索引无唯一约束，MarkAnalyzed 失败
	// 后下轮扫描会对同一激活重复生成报告。写前按 invocation_id 存在性检查，
	// 已存在即跳过（空 invocation_id 不参与去重——非激活来源无法用此键判重）。
	if report.InvocationID != "" {
		exists, err := r.data.RW().Read(ctx).ExperienceReport.Query().
			Where(experiencereport.InvocationIDEQ(report.InvocationID)).
			Exist(ctx)
		if err != nil {
			return entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		if exists {
			r.lg.Info("experience report already exists for invocation, idempotent skip",
				loggateway.StepID("skill_intelligence.create"),
				loggateway.Str("invocation_id", report.InvocationID),
				loggateway.Str("skill_id", report.SkillID))
			return nil
		}
	}
	builder, err := r.reportToCreateBuilder(ctx, report)
	if err != nil {
		return err
	}
	_, err = builder.Save(ctx)
	return entErrToBizErr(err, "SKILL_INTELLIGENCE")
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
	return entErrToBizErr(err, "SKILL_INTELLIGENCE")
}

// ── SkillHealthAggregator ─────────────────────────────────────────────────────

func (r *SkillIntelligenceRepo) GetHealthMetrics(ctx context.Context, skillID string, since time.Time) (*biz.SkillHealthMetrics, error) {
	sinceStr := since.UTC().Format(time.RFC3339)

	// Main aggregation query — computes count, success_count, avg_duration, avg_token_usage in SQL.
	// 审计 W1 统计口径：invocation_count / 成功率分母只计已完成执行
	// （排除 outcome=partial/cancelled 的取消、门禁拒绝、中途记录等非终态中断）；
	// legacy outcome='' 行保留计数（由 success_count 侧的 status 分支判成败）。
	d := r.data.Dialect()
	tokenUsageTotal := d.JSONExtractNumeric("token_usage", "total")
	aggQuery := d.RenumberPlaceholders(`SELECT
  COALESCE(SUM(CASE WHEN outcome NOT IN ('partial', 'cancelled') THEN 1 ELSE 0 END), 0) as invocation_count,
  COALESCE(SUM(CASE WHEN outcome = 'success' OR (outcome = '' AND (status = 'completed' OR status = 'success')) THEN 1 ELSE 0 END), 0) as success_count,
  AVG(CASE WHEN outcome NOT IN ('partial', 'cancelled') AND duration_ms > 0 THEN duration_ms END) as avg_duration_ms,
  COALESCE(AVG(CASE WHEN outcome NOT IN ('partial', 'cancelled') AND token_usage IS NOT NULL THEN ` + tokenUsageTotal + ` END), 0) as avg_token_usage
FROM skill_invocation
WHERE skill_id = ? AND created_at >= ? AND source = 'runtime'`)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, aggQuery, skillID, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	defer rows.Close()

	metrics := &biz.SkillHealthMetrics{SkillID: skillID}
	if rows.Next() {
		var invocationCount, successCount int
		var avgDurationMS sql.NullFloat64
		var avgTokenUsage float64
		if err := rows.Scan(&invocationCount, &successCount, &avgDurationMS, &avgTokenUsage); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		metrics.InvocationCount = invocationCount
		metrics.SuccessCount = successCount
		if avgDurationMS.Valid {
			metrics.AvgDurationMS = avgDurationMS.Float64
		}
		metrics.AvgTokenUsage = int(avgTokenUsage)
		if invocationCount > 0 {
			metrics.SuccessRate = float64(successCount) / float64(invocationCount)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	// P95: for small row counts (<20), load durations and compute in Go;
	// for larger sets, use SQL ORDER BY + OFFSET approach.
	countQuery := d.RenumberPlaceholders(`SELECT COUNT(*) FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0 AND source = 'runtime'`)
	var durCount int
	cRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, countQuery, skillID, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	defer cRows.Close()
	if cRows.Next() {
		if err := cRows.Scan(&durCount); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
	}
	if err := cRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	if durCount == 0 {
		metrics.P95DurationMS = 0
	} else if durCount < 20 {
		// Small set: load all durations and compute P95 in Go.
		durQuery := d.RenumberPlaceholders(`SELECT duration_ms FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0 AND source = 'runtime' ORDER BY duration_ms ASC`)
		dRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, durQuery, skillID, sinceStr)
		if err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		defer dRows.Close()
		var durations []int
		for dRows.Next() {
			var d int
			if err := dRows.Scan(&d); err != nil {
				return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
			}
			durations = append(durations, d)
		}
		if err := dRows.Err(); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		metrics.P95DurationMS = types.P95(durations)
	} else {
		// Large set: use SQL OFFSET to pick the 95th percentile row.
		offset := durCount * 95 / 100
		if offset >= durCount {
			offset = durCount - 1
		}
		p95Query := d.RenumberPlaceholders(`SELECT duration_ms FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND duration_ms > 0 AND source = 'runtime' ORDER BY duration_ms ASC LIMIT 1 OFFSET ?`)
		var p95 int
		pRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, p95Query, skillID, sinceStr, offset)
		if err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		defer pRows.Close()
		if pRows.Next() {
			if err := pRows.Scan(&p95); err != nil {
				return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
			}
		}
		if err := pRows.Err(); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
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
	d := r.data.Dialect()

	// Count total failures. 与 GetHealthMetrics 口径一致：只统计真实运行时调用。
	// 审计 W1：partial/cancelled（取消、门禁拒绝、中途记录）不计失败——
	// 它们不代表技能执行失败，纳入会虚增失败率并误触发失败推动化。
	failCountQuery := d.RenumberPlaceholders(`SELECT COUNT(*) FROM skill_invocation WHERE skill_id = ? AND created_at >= ? AND source = 'runtime' AND outcome != 'success' AND outcome NOT IN ('partial', 'cancelled') AND NOT (outcome = '' AND (status = 'completed' OR status = 'success'))`)
	var failureCount int
	fRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, failCountQuery, skillID, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	defer fRows.Close()
	if fRows.Next() {
		if err := fRows.Scan(&failureCount); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
	}
	if err := fRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	// Top error codes via SQL GROUP BY.
	topCodesQuery := d.RenumberPlaceholders(`SELECT
  COALESCE(error_code, 'unknown') as error_code,
  COUNT(*) as count
FROM skill_invocation
WHERE skill_id = ? AND created_at >= ? AND source = 'runtime' AND outcome != 'success' AND outcome NOT IN ('partial', 'cancelled') AND NOT (outcome = '' AND (status = 'completed' OR status = 'success'))
GROUP BY error_code
ORDER BY count DESC
LIMIT 5`)
	cRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, topCodesQuery, skillID, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	defer cRows.Close()

	var topCodes []biz.ErrorCodeCount
	for cRows.Next() {
		var code string
		var count int
		if err := cRows.Scan(&code, &count); err != nil {
			return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
		}
		topCodes = append(topCodes, biz.ErrorCodeCount{ErrorCode: code, Count: count})
	}
	if err := cRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}

	return &biz.SkillFailureStats{
		SkillID:       skillID,
		FailureCount:  failureCount,
		TopErrorCodes: topCodes,
	}, nil
}

// GetFailureTagCounts returns failure tag counts for a skill over the given
// time window by querying experience reports.
// 审计 T8：与 GetFailureTagCountsFiltered 同口径下沉 SQL（见 queryFailureTagCounts）。
func (r *SkillIntelligenceRepo) GetFailureTagCounts(ctx context.Context, skillID string, since time.Time) ([]biz.FailureTagCount, error) {
	conds := []string{"skill_id = ?", "created_at >= ?", "is_success = false"}
	args := []any{skillID, since.UTC().Format(time.RFC3339)}
	return r.queryFailureTagCounts(ctx, conds, args)
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

// maxSkillAnalysisAttempts 审计 W5②：报告分析失败重试上限。GenerateReport 失败
// 时 RecordAnalysisFailure 自增 analysis_attempts；达到上限的行被下方谓词排除
// （死信隔离，analysis_error 保留最近错误供排查后经 SQL 清零复活），根治毒行
// 「按 created_at ASC 每轮重扫 + 积压满批次后队头阻塞」。15min 一轮 = 最多再
// 试约 1.25 小时，对瞬时故障足够宽容。
const maxSkillAnalysisAttempts = 5

// analysisErrorMaxLen 落库的最近错误最大长度（防超长错误链路占行宽）。
const analysisErrorMaxLen = 500

func (r *SkillIntelligenceRepo) ListUnanalyzed(ctx context.Context, batchSize int) ([]biz.SkillInvocationWrite, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.AnalyzedAtEQ(""),
			skillinvocation.OutcomeNEQ(""),
			// 审计 W1：partial/cancelled 是非终态中断，不是成败信号，
			// 不生成经验报告（否则取消会被归因 unknown，污染报告统计）。
			skillinvocation.OutcomeNotIn("partial", "cancelled"),
			// 审计 W5②：死信隔离——失败次数达上限的毒行不再进入批次。
			skillinvocation.AnalysisAttemptsLT(maxSkillAnalysisAttempts),
		).
		Order(ent.Asc(skillinvocation.FieldCreatedAt)).
		Limit(batchSize).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	result := make([]biz.SkillInvocationWrite, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapEntSkillInvocationToWrite(row))
	}
	// skill_invocation 不冗余存 skill_name；报告生成依赖 SkillName（FN-7），
	// 此处批量联表 platform_skill 回填，保证新经验报告携带技能名。
	r.backfillInvocationSkillNames(ctx, result)
	return result, nil
}

// backfillInvocationSkillNames fills SkillName on invocation writes via a single
// batch lookup against platform_skill. Best-effort: lookup failure leaves names empty.
func (r *SkillIntelligenceRepo) backfillInvocationSkillNames(ctx context.Context, writes []biz.SkillInvocationWrite) {
	idSet := map[string]struct{}{}
	for _, w := range writes {
		if w.SkillID != "" && w.SkillName == "" {
			idSet[w.SkillID] = struct{}{}
		}
	}
	names := r.querySkillNames(ctx, idSet)
	for i := range writes {
		if writes[i].SkillName == "" {
			writes[i].SkillName = names[writes[i].SkillID]
		}
	}
}

// backfillReportSkillNames fills SkillName on reports whose stored skill_name is
// empty (legacy rows written before the backfill existed). Best-effort.
func (r *SkillIntelligenceRepo) backfillReportSkillNames(ctx context.Context, reports []biz.ExperienceReport) {
	idSet := map[string]struct{}{}
	for _, rep := range reports {
		if rep.SkillID != "" && rep.SkillName == "" {
			idSet[rep.SkillID] = struct{}{}
		}
	}
	names := r.querySkillNames(ctx, idSet)
	for i := range reports {
		if reports[i].SkillName == "" {
			reports[i].SkillName = names[reports[i].SkillID]
		}
	}
}

// querySkillNames batch-resolves platform_skill IDs to display names.
// Returns an empty map (not error) on lookup failure — name backfill is best-effort.
func (r *SkillIntelligenceRepo) querySkillNames(ctx context.Context, idSet map[string]struct{}) map[string]string {
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDIn(ids...)).
		Select(platformskill.FieldID, platformskill.FieldName).
		All(ctx)
	if err != nil {
		r.lg.Warn("backfill skill names failed", loggateway.StepID("data.skill_intelligence"), loggateway.Err(err))
		return nil
	}
	names := make(map[string]string, len(skills))
	for _, s := range skills {
		names[s.ID] = s.Name
	}
	return names
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
		return entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	if n == 0 {
		return apierror.NotFound("SKILL_INTELLIGENCE", fmt.Sprintf("skill invocation with activation_id %s not found", activationID))
	}
	return nil
}

// RecordAnalysisFailure 审计 W5②：记录一次报告生成失败——analysis_attempts 自增
// 并落最近错误（截断防超长）。达到 maxSkillAnalysisAttempts 的行被 ListUnanalyzed
// 死信隔离；行保持 analyzed_at=''，成绩状态清晰可查（attempts + analysis_error）。
// 与 MarkAnalyzed 同款的 activation_id 定位 + n==0 NotFound 语义。
func (r *SkillIntelligenceRepo) RecordAnalysisFailure(ctx context.Context, activationID, errMsg string) error {
	if activationID == "" {
		return nil
	}
	errMsg = truncateUTF8(errMsg, analysisErrorMaxLen)
	n, err := r.data.RW().Write(ctx).SkillInvocation.Update().
		Where(skillinvocation.ActivationIDEQ(activationID)).
		AddAnalysisAttempts(1).
		SetAnalysisError(errMsg).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	if n == 0 {
		return apierror.NotFound("SKILL_INTELLIGENCE", fmt.Sprintf("skill invocation with activation_id %s not found", activationID))
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
