package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/patchoutcome"
	"aranea-agents/internal/data/ent/selfimprovementrun"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// SelfImprovementRunRepo implements biz.SelfImprovementRunReader /
// biz.SelfImprovementRunWriter / biz.PatchOutcomeWriter over Ent.
type SelfImprovementRunRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.SelfImprovementRunReader = (*SelfImprovementRunRepo)(nil)
var _ biz.SelfImprovementRunWriter = (*SelfImprovementRunRepo)(nil)
var _ biz.PatchOutcomeWriter = (*SelfImprovementRunRepo)(nil)
var _ biz.PatchOutcomeStatsReader = (*SelfImprovementRunRepo)(nil)

// NewSelfImprovementRunRepo creates the repo. Logger is held independently
// (data 层推荐模式).
func NewSelfImprovementRunRepo(d *Data, lg loggateway.Logger) *SelfImprovementRunRepo {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementRunRepo{data: d, lg: lg}
}

// ── JSON conversion helpers (json round-trip between typed structs and Ent
// map/slice JSON columns) ──

func siStructToMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func siMapToStruct(m map[string]any, v any) error {
	if m == nil {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// ── Create / Read ──

func (r *SelfImprovementRunRepo) Create(ctx context.Context, run *biz.SelfImprovementRun) error {
	if r == nil || r.data == nil {
		return apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	diffStats := map[string]int{
		"files": run.DiffStats.Files, "additions": run.DiffStats.Additions, "deletions": run.DiffStats.Deletions,
	}
	c := r.data.RW().Write(ctx).SelfImprovementRun.Create().
		SetID(run.ID).
		SetSuggestionID(run.SuggestionID).
		SetStatus(string(run.Status)).
		SetTriggerSource(run.TriggerSource).
		SetPatchKind(string(run.PatchKind)).
		SetRiskLevel(string(run.RiskLevel)).
		SetBaseRef(run.BaseRef).
		SetBranch(run.Branch).
		SetWorktreePath(run.WorktreePath).
		SetDiff(run.Diff).
		SetDiffStats(diffStats).
		SetAttempts(run.Attempts).
		SetApprovedBy(run.ApprovedBy).
		SetAppliedCommit(run.AppliedCommit).
		SetRollbackPointer(run.RollbackPointer).
		SetClosedReason(run.ClosedReason)
	if run.Diagnosis != nil {
		m, err := siStructToMap(run.Diagnosis)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetDiagnosis(m)
	}
	if len(run.VerificationReport) > 0 {
		raw, err := json.Marshal(run.VerificationReport)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		var report []map[string]any
		if err := json.Unmarshal(raw, &report); err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetVerificationReport(report)
	}
	if run.CriticReport != nil {
		m, err := siStructToMap(run.CriticReport)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetCriticReport(m)
	}
	if run.Governance != nil {
		m, err := siStructToMap(run.Governance)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetGovernance(m)
	}
	if run.ObserveUntil != nil {
		c.SetObserveUntil(*run.ObserveUntil)
	}
	if len(run.Metadata) > 0 {
		var m map[string]any
		if err := json.Unmarshal(run.Metadata, &m); err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetMetadata(m)
	}
	if _, err := c.Save(ctx); err != nil {
		return entErrToBizErr(err, "SELF_IMPROVE")
	}
	return nil
}

func (r *SelfImprovementRunRepo) GetByID(ctx context.Context, id string) (*biz.SelfImprovementRun, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	row, err := r.data.RW().Read(ctx).SelfImprovementRun.Query().
		Where(selfimprovementrun.ID(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	return siRunToBiz(row)
}

func (r *SelfImprovementRunRepo) GetBySuggestionID(ctx context.Context, suggestionID string) (*biz.SelfImprovementRun, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	row, err := r.data.RW().Read(ctx).SelfImprovementRun.Query().
		Where(selfimprovementrun.SuggestionID(suggestionID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	return siRunToBiz(row)
}

func (r *SelfImprovementRunRepo) List(ctx context.Context, filter biz.RunFilter) ([]biz.SelfImprovementRun, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	q := r.data.RW().Read(ctx).SelfImprovementRun.Query().
		Order(ent.Desc(selfimprovementrun.FieldCreatedAt))
	if filter.Status != "" {
		q = q.Where(selfimprovementrun.StatusEQ(string(filter.Status)))
	}
	if len(filter.Statuses) > 0 {
		q = q.Where(selfimprovementrun.StatusIn(siRunStatusStrings(filter.Statuses)...))
	}
	if filter.RiskLevel != "" {
		q = q.Where(selfimprovementrun.RiskLevelEQ(string(filter.RiskLevel)))
	}
	if filter.TriggerSource != "" {
		q = q.Where(selfimprovementrun.TriggerSourceEQ(filter.TriggerSource))
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	out := make([]biz.SelfImprovementRun, 0, len(rows))
	for _, row := range rows {
		b, err := siRunToBiz(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, nil
}

// Count implements biz.SelfImprovementRunReader: same filter conditions as
// List, ignoring Limit/Offset (console list total, P5).
func (r *SelfImprovementRunRepo) Count(ctx context.Context, filter biz.RunFilter) (int, error) {
	if r == nil || r.data == nil {
		return 0, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	q := r.data.RW().Read(ctx).SelfImprovementRun.Query()
	if filter.Status != "" {
		q = q.Where(selfimprovementrun.StatusEQ(string(filter.Status)))
	}
	if len(filter.Statuses) > 0 {
		q = q.Where(selfimprovementrun.StatusIn(siRunStatusStrings(filter.Statuses)...))
	}
	if filter.RiskLevel != "" {
		q = q.Where(selfimprovementrun.RiskLevelEQ(string(filter.RiskLevel)))
	}
	if filter.TriggerSource != "" {
		q = q.Where(selfimprovementrun.TriggerSourceEQ(filter.TriggerSource))
	}
	n, err := q.Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "SELF_IMPROVE")
	}
	return n, nil
}

// siRunStatusStrings converts the biz status slice for Ent StatusIn predicates.
func siRunStatusStrings(statuses []biz.SelfImprovementRunStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, string(s))
	}
	return out
}

// ListTerminalPendingOutcome implements biz.SelfImprovementRunReader: terminal
// runs without any PatchOutcome, oldest first (Outcome worker attribution).
func (r *SelfImprovementRunRepo) ListTerminalPendingOutcome(ctx context.Context, limit int) ([]biz.SelfImprovementRun, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	// run IDs already attributed (dupes are harmless inside NOT IN).
	attributed, err := r.data.RW().Read(ctx).PatchOutcome.Query().
		Select(patchoutcome.FieldRunID).
		Strings(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	q := r.data.RW().Read(ctx).SelfImprovementRun.Query().
		Where(selfimprovementrun.StatusIn(
			string(biz.RunStatusClosed),
			string(biz.RunStatusRolledBack),
			string(biz.RunStatusVerifyFailed),
			string(biz.RunStatusRejected),
			string(biz.RunStatusFailed),
		)).
		Order(ent.Asc(selfimprovementrun.FieldCreatedAt)).
		Limit(limit)
	if len(attributed) > 0 {
		q = q.Where(selfimprovementrun.IDNotIn(attributed...))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	out := make([]biz.SelfImprovementRun, 0, len(rows))
	for _, row := range rows {
		b, err := siRunToBiz(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, nil
}

// ── Update (CAS on from-status) / Attempts ──

func (r *SelfImprovementRunRepo) Update(ctx context.Context, run *biz.SelfImprovementRun, from biz.SelfImprovementRunStatus) error {
	if r == nil || r.data == nil {
		return apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	diffStats := map[string]int{
		"files": run.DiffStats.Files, "additions": run.DiffStats.Additions, "deletions": run.DiffStats.Deletions,
	}
	u := r.data.RW().Write(ctx).SelfImprovementRun.Update().
		Where(
			selfimprovementrun.ID(run.ID),
			selfimprovementrun.StatusEQ(string(from)),
		).
		SetStatus(string(run.Status)).
		SetPatchKind(string(run.PatchKind)).
		SetRiskLevel(string(run.RiskLevel)).
		SetBaseRef(run.BaseRef).
		SetBranch(run.Branch).
		SetWorktreePath(run.WorktreePath).
		SetDiff(run.Diff).
		SetDiffStats(diffStats).
		SetApprovedBy(run.ApprovedBy).
		SetAppliedCommit(run.AppliedCommit).
		SetRollbackPointer(run.RollbackPointer).
		SetClosedReason(run.ClosedReason).
		SetUpdatedAt(time.Now().UTC())

	diagnosis := map[string]any{}
	if run.Diagnosis != nil {
		m, err := siStructToMap(run.Diagnosis)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		diagnosis = m
	}
	u.SetDiagnosis(diagnosis)

	report := make([]map[string]any, 0, len(run.VerificationReport))
	if len(run.VerificationReport) > 0 {
		raw, err := json.Marshal(run.VerificationReport)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
	}
	u.SetVerificationReport(report)

	critic := map[string]any{}
	if run.CriticReport != nil {
		m, err := siStructToMap(run.CriticReport)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		critic = m
	}
	u.SetCriticReport(critic)

	governance := map[string]any{}
	if run.Governance != nil {
		m, err := siStructToMap(run.Governance)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		governance = m
	}
	u.SetGovernance(governance)

	if run.ObserveUntil != nil {
		u.SetObserveUntil(*run.ObserveUntil)
	} else {
		u.ClearObserveUntil()
	}

	// Metadata 全量覆盖（与 diagnosis/critic/governance 同语义：调用方持有的
	// run 由 GetByID/List 载入，Metadata 即当前真相）。缺失此映射曾导致
	// watchdog 基线/到期快照永不落库（run 永卡 observing）。
	metadata := map[string]any{}
	if len(run.Metadata) > 0 {
		if err := json.Unmarshal(run.Metadata, &metadata); err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
	}
	u.SetMetadata(metadata)

	n, err := u.Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "SELF_IMPROVE")
	}
	if n == 0 {
		return apierror.Conflict("SELF_IMPROVE", "run %s status conflict (expected %s)", run.ID, from)
	}
	return nil
}

func (r *SelfImprovementRunRepo) RecordAttempt(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	if _, err := r.data.RW().Write(ctx).SelfImprovementRun.UpdateOneID(id).
		AddAttempts(1).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		return entErrToBizErr(err, "SELF_IMPROVE")
	}
	return nil
}

// ── PatchOutcome ──

func (r *SelfImprovementRunRepo) CreateOutcome(ctx context.Context, outcome *biz.PatchOutcome) error {
	if r == nil || r.data == nil {
		return apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	c := r.data.RW().Write(ctx).PatchOutcome.Create().
		SetID(outcome.ID).
		SetRunID(outcome.RunID).
		SetSuggestionID(outcome.SuggestionID).
		SetVerdict(string(outcome.Verdict)).
		SetRollbackReason(outcome.RollbackReason).
		SetPatternHash(outcome.PatternHash)
	if outcome.MetricsBefore != nil {
		m, err := siStructToMap(outcome.MetricsBefore)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetMetricsBefore(m)
	}
	if outcome.MetricsAfter != nil {
		m, err := siStructToMap(outcome.MetricsAfter)
		if err != nil {
			return entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.SetMetricsAfter(m)
	}
	if _, err := c.Save(ctx); err != nil {
		return entErrToBizErr(err, "SELF_IMPROVE")
	}
	return nil
}

func (r *SelfImprovementRunRepo) ListOutcomesByRun(ctx context.Context, runID string) ([]biz.PatchOutcome, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	rows, err := r.data.RW().Read(ctx).PatchOutcome.Query().
		Where(patchoutcome.RunID(runID)).
		Order(ent.Desc(patchoutcome.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	out := make([]biz.PatchOutcome, 0, len(rows))
	for _, row := range rows {
		o, err := siOutcomeToBiz(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

// ListRecentOutcomesByTrigger implements biz.PatchOutcomeWriter: newest
// outcomes for one trigger source (join through runs), newest first.
func (r *SelfImprovementRunRepo) ListRecentOutcomesByTrigger(ctx context.Context, triggerSource string, limit int) ([]biz.PatchOutcome, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	runIDs, err := r.data.RW().Read(ctx).SelfImprovementRun.Query().
		Where(selfimprovementrun.TriggerSourceEQ(triggerSource)).
		Select(selfimprovementrun.FieldID).
		Strings(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	if len(runIDs) == 0 {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).PatchOutcome.Query().
		Where(patchoutcome.RunIDIn(runIDs...)).
		Order(ent.Desc(patchoutcome.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	out := make([]biz.PatchOutcome, 0, len(rows))
	for _, row := range rows {
		o, err := siOutcomeToBiz(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

// AggregateOutcomeStats implements biz.PatchOutcomeStatsReader: per
// (trigger_source, verdict) counts of patch_outcomes joined through runs
// (console stats panel, P5). Raw SQL per DB-N2 (Ent cannot express the
// cross-table GROUP BY).
func (r *SelfImprovementRunRepo) AggregateOutcomeStats(ctx context.Context) ([]biz.SITriggerVerdictCount, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, `
		SELECT r.trigger_source, o.verdict, COUNT(*)::int
		FROM patch_outcomes o
		JOIN self_improvement_runs r ON r.id = o.run_id
		GROUP BY r.trigger_source, o.verdict`)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	defer rows.Close()
	out := []biz.SITriggerVerdictCount{}
	for rows.Next() {
		var c biz.SITriggerVerdictCount
		if err := rows.Scan(&c.TriggerSource, &c.Verdict, &c.Count); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	return out, nil
}

// siOutcomeToBiz converts one Ent PatchOutcome row to the biz model.
func siOutcomeToBiz(row *ent.PatchOutcome) (*biz.PatchOutcome, error) {
	o := &biz.PatchOutcome{
		ID:             row.ID,
		RunID:          row.RunID,
		SuggestionID:   row.SuggestionID,
		Verdict:        biz.SelfImprovementVerdict(row.Verdict),
		RollbackReason: row.RollbackReason,
		PatternHash:    row.PatternHash,
		CreatedAt:      row.CreatedAt,
	}
	if row.MetricsBefore != nil {
		snap := &biz.MetricsSnapshot{}
		if err := siMapToStruct(row.MetricsBefore, snap); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		o.MetricsBefore = snap
	}
	if row.MetricsAfter != nil {
		snap := &biz.MetricsSnapshot{}
		if err := siMapToStruct(row.MetricsAfter, snap); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		o.MetricsAfter = snap
	}
	return o, nil
}

// ── Ent → Biz conversion ──

func siRunToBiz(row *ent.SelfImprovementRun) (*biz.SelfImprovementRun, error) {
	run := &biz.SelfImprovementRun{
		ID:              row.ID,
		SuggestionID:    row.SuggestionID,
		Status:          biz.SelfImprovementRunStatus(row.Status),
		TriggerSource:   row.TriggerSource,
		PatchKind:       biz.SelfImprovementPatchKind(row.PatchKind),
		RiskLevel:       biz.SelfImprovementRiskLevel(row.RiskLevel),
		BaseRef:         row.BaseRef,
		Branch:          row.Branch,
		WorktreePath:    row.WorktreePath,
		Diff:            row.Diff,
		Attempts:        row.Attempts,
		ApprovedBy:      row.ApprovedBy,
		AppliedCommit:   row.AppliedCommit,
		RollbackPointer: row.RollbackPointer,
		ClosedReason:    row.ClosedReason,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.DiffStats != nil {
		run.DiffStats = biz.DiffStats{
			Files:     row.DiffStats["files"],
			Additions: row.DiffStats["additions"],
			Deletions: row.DiffStats["deletions"],
		}
	}
	// 空 map / 零值结构体均视为未设置：Update 全量覆盖在 biz 字段为 nil 时写入
	// {}（ent JSON 列非 Nillable，无 NULL 语义）；历史读路径把 {} 物化为零值
	// 结构体后被再次全量覆盖写回，升级为带完整零值字段的 map。两种污染形态
	// 都不可能来自真实生产路径（ParseDiagnosisJSON 强制 root_cause；
	// ParseCriticReportJSON 强制 risk_level 枚举；SIRiskClassifier 恒设
	// channel），读到即视为该阶段未执行——否则零值 CriticReport{IsSafe:false}
	// 经 API 透出后被前端误渲染为「存在风险」。
	if len(row.Diagnosis) > 0 {
		d := &biz.Diagnosis{}
		if err := siMapToStruct(row.Diagnosis, d); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		if d.RootCause != "" {
			run.Diagnosis = d
		}
	}
	if row.VerificationReport != nil {
		raw, err := json.Marshal(row.VerificationReport)
		if err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		var report []biz.SandboxGateResult
		if err := json.Unmarshal(raw, &report); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		run.VerificationReport = report
	}
	if len(row.CriticReport) > 0 {
		c := &biz.CriticReport{}
		if err := siMapToStruct(row.CriticReport, c); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		if c.RiskLevel != "" {
			run.CriticReport = c
		}
	}
	if len(row.Governance) > 0 {
		g := &biz.GovernanceDecision{}
		if err := siMapToStruct(row.Governance, g); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		if g.Channel != "" {
			run.Governance = g
		}
	}
	if row.ObserveUntil != nil {
		t := *row.ObserveUntil
		run.ObserveUntil = &t
	}
	if row.Metadata != nil {
		raw, err := json.Marshal(row.Metadata)
		if err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		run.Metadata = raw
	}
	return run, nil
}
