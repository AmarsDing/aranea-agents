package biz

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/bmatcuk/doublestar/v4"
)

// ── Admin manual entries (73-self-iteration-v3, design D7 / §7, T4.3) ───────
//
// SelfImprovementAdminUsecase is the operator-facing manual control surface:
//
//	Approve  — awaiting_governance → applying（记 ApprovedBy，挂钩 ApplyDriver）
//	Reject   — awaiting_governance → rejected（reason 必填）
//	Rollback — applied/observing → rolled_back（经 SIApplier 实际回退后 CAS 迁移）
//	Close    — observing → closed（提前确认有效；applied 须先入观察窗）
//
// P4 仅落 biz 用例（管理 API 内部路径由 service 绑定）；P5 落 Proto
// （ApproveRun/RejectRun/RollbackRun/CloseRun，design §7）。状态机校验 +
// CAS Update 与自动路径（Watchdog/Drive）天然互斥：迁移冲突即「他入口已
// 推进」，向外返回 Conflict。

// SelfImprovementAdminDeps carries the admin usecase's injected deps.
type SelfImprovementAdminDeps struct {
	RunReader SelfImprovementRunReader
	RunWriter SelfImprovementRunWriter
	Applier   SIApplier
	// ApplyDriver nil → approve 迁移到 applying 后由 drive worker 接管。
	ApplyDriver SIApplyDriver
	// StatsReader nil → OutcomeStats 返回 Internal（console 部分降级）。
	StatsReader PatchOutcomeStatsReader
	// RiskRules nil → GetRiskRules 返回代码默认、UpdateRiskRules 返回 Internal。
	RiskRules SIRiskRuleRepo
	Lg        loggateway.Logger
}

// SelfImprovementAdminUsecase executes operator manual actions on runs.
type SelfImprovementAdminUsecase struct {
	runReader  SelfImprovementRunReader
	runWriter  SelfImprovementRunWriter
	applier    SIApplier
	applyDrive SIApplyDriver
	stats      PatchOutcomeStatsReader
	riskRules  SIRiskRuleRepo
	lg         loggateway.Logger
}

// NewSelfImprovementAdminUsecase wires the admin usecase.
func NewSelfImprovementAdminUsecase(deps SelfImprovementAdminDeps) (*SelfImprovementAdminUsecase, error) {
	if deps.RunReader == nil || deps.RunWriter == nil || deps.Applier == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "run reader/writer and applier are required")
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementAdminUsecase{
		runReader: deps.RunReader, runWriter: deps.RunWriter,
		applier: deps.Applier, applyDrive: deps.ApplyDriver,
		stats: deps.StatsReader, riskRules: deps.RiskRules,
		lg: lg.With(loggateway.Domain("self_improve_admin")),
	}, nil
}

// ── Console query surface (P5, design §7 ListRuns/GetRun/GetOutcomeStats) ────

// SITriggerOutcomeStats aggregates verdicts of one trigger source.
type SITriggerOutcomeStats struct {
	TriggerSource string
	Total         int
	Effective     int
	Neutral       int
	Regressed     int
}

// SIOutcomeStats is the console outcome-stats view (GetOutcomeStats RPC).
type SIOutcomeStats struct {
	Total         int
	Effective     int
	Neutral       int
	Regressed     int
	EffectiveRate float64 // effective / total（total=0 → 0）
	RollbackRate  float64 // regressed / total（total=0 → 0）
	ByTrigger     []SITriggerOutcomeStats
}

// List returns runs matching the filter plus the un-paginated total
// (console list, design §7 ListRuns).
func (uc *SelfImprovementAdminUsecase) List(ctx context.Context, filter RunFilter) ([]SelfImprovementRun, int, error) {
	runs, err := uc.runReader.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.runReader.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

// Get returns one run's full detail or NotFound (design §7 GetRun).
func (uc *SelfImprovementAdminUsecase) Get(ctx context.Context, runID string) (*SelfImprovementRun, error) {
	return uc.mustGet(ctx, runID)
}

// OutcomeStats aggregates terminal attribution records into the console stats
// view (design §7 GetOutcomeStats): totals + per-trigger breakdown + rates.
// ByTrigger is sorted by trigger source for deterministic rendering.
func (uc *SelfImprovementAdminUsecase) OutcomeStats(ctx context.Context) (*SIOutcomeStats, error) {
	if uc.stats == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "outcome stats reader not wired")
	}
	rows, err := uc.stats.AggregateOutcomeStats(ctx)
	if err != nil {
		return nil, err
	}
	out := &SIOutcomeStats{}
	byTrigger := map[string]*SITriggerOutcomeStats{}
	for _, row := range rows {
		out.Total += row.Count
		trg, ok := byTrigger[row.TriggerSource]
		if !ok {
			trg = &SITriggerOutcomeStats{TriggerSource: row.TriggerSource}
			byTrigger[row.TriggerSource] = trg
		}
		trg.Total += row.Count
		switch row.Verdict {
		case VerdictEffective:
			out.Effective += row.Count
			trg.Effective += row.Count
		case VerdictNeutral:
			out.Neutral += row.Count
			trg.Neutral += row.Count
		case VerdictRegressed:
			out.Regressed += row.Count
			trg.Regressed += row.Count
		}
	}
	if out.Total > 0 {
		out.EffectiveRate = float64(out.Effective) / float64(out.Total)
		out.RollbackRate = float64(out.Regressed) / float64(out.Total)
	}
	out.ByTrigger = make([]SITriggerOutcomeStats, 0, len(byTrigger))
	for _, trg := range byTrigger {
		out.ByTrigger = append(out.ByTrigger, *trg)
	}
	sort.Slice(out.ByTrigger, func(i, j int) bool {
		return out.ByTrigger[i].TriggerSource < out.ByTrigger[j].TriggerSource
	})
	return out, nil
}

// ── Risk rules (P5, design §7 UpdateRiskRules) ───────────────────────────────

// GetRiskRules returns the configured risk rules; zero fields mean "inherit
// code defaults" (the console renders them as the effective defaults). When
// the repo is not wired the code defaults are returned directly.
func (uc *SelfImprovementAdminUsecase) GetRiskRules(ctx context.Context) (SIRiskRules, error) {
	if uc.riskRules == nil {
		return DefaultSIRiskRules(), nil
	}
	return uc.riskRules.GetSIRiskRules(ctx)
}

// UpdateRiskRules validates and persists the admin-configured rule set.
// Zero-valued fields reset to the code defaults on next load (storage keeps
// the raw values; consumers normalize).
func (uc *SelfImprovementAdminUsecase) UpdateRiskRules(ctx context.Context, operator string, rules SIRiskRules) (SIRiskRules, error) {
	if uc.riskRules == nil {
		return SIRiskRules{}, apierror.Internal("SELF_IMPROVEMENT", "risk rule repo not wired")
	}
	if err := validateSIRiskRules(rules); err != nil {
		return SIRiskRules{}, err
	}
	saved, err := uc.riskRules.UpdateSIRiskRules(ctx, rules)
	if err != nil {
		return SIRiskRules{}, err
	}
	uc.lg.Info("self-improve risk rules updated by operator",
		loggateway.StepID("si_admin.risk_rules"),
		loggateway.Str("operator", operator),
		loggateway.Int("low_max_lines", rules.LowMaxLines),
		loggateway.Int("medium_max_lines", rules.MediumMaxLines),
		loggateway.Int("daily_auto_quota", int(rules.DailyAutoQuota)),
		loggateway.Int("core_path_globs", len(rules.CorePathGlobs)))
	return saved, nil
}

// validateSIRiskRules enforces the console-boundary invariants: thresholds
// are either 0 (inherit default) or positive, low ≤ medium when both set,
// quota non-negative, and every glob is a valid doublestar pattern.
func validateSIRiskRules(rules SIRiskRules) error {
	if rules.LowMaxLines < 0 || rules.MediumMaxLines < 0 || rules.DailyAutoQuota < 0 {
		return apierror.BadRequest("SELF_IMPROVEMENT", "risk rule thresholds must be >= 0 (0 = inherit default)")
	}
	if rules.LowMaxLines > 0 && rules.MediumMaxLines > 0 && rules.LowMaxLines > rules.MediumMaxLines {
		return apierror.BadRequest("SELF_IMPROVEMENT", "low_max_lines (%d) must be <= medium_max_lines (%d)", rules.LowMaxLines, rules.MediumMaxLines)
	}
	for _, glob := range rules.CorePathGlobs {
		if strings.TrimSpace(glob) == "" {
			return apierror.BadRequest("SELF_IMPROVEMENT", "core_path_globs must not contain blank entries")
		}
		if !doublestar.ValidatePattern(glob) {
			return apierror.BadRequest("SELF_IMPROVEMENT", "invalid core_path_globs pattern %q", glob)
		}
	}
	return nil
}

// Approve transitions an awaiting_governance run to applying (operator
// approval of the high-risk channel) and kicks the apply driver.
func (uc *SelfImprovementAdminUsecase) Approve(ctx context.Context, runID, operator, reason string) error {
	run, err := uc.mustGet(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusAwaitingGovernance {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not awaiting_governance (current %s)", runID, run.Status)
	}
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventApply)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --apply--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.ApprovedBy = operator
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusAwaitingGovernance); err != nil {
		return err
	}
	uc.lg.Info("self-improve run approved by operator",
		loggateway.StepID("si_admin.approve"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("operator", operator))
	if uc.applyDrive != nil {
		if err := uc.applyDrive.Apply(ctx, run.ID); err != nil {
			return err // 迁移已生效；apply 失败由 drive worker 下 tick 重驱动。
		}
	}
	return nil
}

// Reject terminates an awaiting_governance run as rejected (reason required,
// design §7 RejectRun).
func (uc *SelfImprovementAdminUsecase) Reject(ctx context.Context, runID, operator, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return apierror.BadRequest("SELF_IMPROVEMENT", "reject reason is required")
	}
	run, err := uc.mustGet(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusAwaitingGovernance {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not awaiting_governance (current %s)", runID, run.Status)
	}
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventReject)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --reject--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.ClosedReason = siTruncateClosedReason("rejected by " + operator + ": " + reason)
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusAwaitingGovernance); err != nil {
		return err
	}
	uc.lg.Info("self-improve run rejected by operator",
		loggateway.StepID("si_admin.reject"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("operator", operator))
	return nil
}

// Rollback manually reverts an applied/observing run (D7 手动控制). The
// applier performs the actual revert before the CAS transition; a revert
// failure leaves the run untouched and propagates.
func (uc *SelfImprovementAdminUsecase) Rollback(ctx context.Context, runID, operator, reason string) error {
	run, err := uc.mustGet(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusApplied && run.Status != RunStatusObserving {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not applied/observing (current %s)", runID, run.Status)
	}
	if strings.TrimSpace(reason) == "" {
		reason = "manual rollback"
	}
	if err := uc.applier.Rollback(ctx, run, reason); err != nil {
		return err
	}
	from := run.Status
	to, err := NewSelfImprovementRunStateMachine().Transition(from, RunEventRollback)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --rollback--> ?: %s", from, err)
	}
	run.Status = to
	run.ClosedReason = siTruncateClosedReason("manual rollback by " + operator)
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, from); err != nil {
		return err
	}
	uc.lg.Info("self-improve run rolled back by operator",
		loggateway.StepID("si_admin.rollback"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("operator", operator),
		loggateway.Str("reason", reason))
	return nil
}

// Close terminates an observing run early (提前确认有效, D7 手动控制).
func (uc *SelfImprovementAdminUsecase) Close(ctx context.Context, runID, operator string) error {
	run, err := uc.mustGet(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusObserving {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not observing (current %s)", runID, run.Status)
	}
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventClose)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --close--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.ClosedReason = siTruncateClosedReason("manual close by " + operator)
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusObserving); err != nil {
		return err
	}
	uc.lg.Info("self-improve run closed by operator",
		loggateway.StepID("si_admin.close"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("operator", operator))
	return nil
}

// mustGet loads a run or maps absence to NotFound.
func (uc *SelfImprovementAdminUsecase) mustGet(ctx context.Context, runID string) (*SelfImprovementRun, error) {
	run, err := uc.runReader.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, apierror.NotFound("SELF_IMPROVEMENT", "run %s not found", runID)
	}
	return run, nil
}
