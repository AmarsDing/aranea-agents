package biz

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Governance router (73-self-iteration-v3, design D6/D10) ─────────────────
//
// SIGovernanceRouter consumes runs left in awaiting_governance by the pipeline
// (govern stage) and routes them by the GovernanceDecision channel:
//
//	auto     → 消耗日配额 → applying（ApprovedBy=system:auto）
//	notify   → 消耗日配额 → applying + 管理员通知（ApprovedBy=system:auto+notify）
//	approval → 提交审批，停留 awaiting_governance
//	reject   → rejected（终态）
//
// 日配额（D10：自动应用 5 次/日）对 auto/notify 两种自动应用通道统一计数，
// 超限一律升级为 approval（转人工），复用 V2 日配额计数模式（24h 窗口重置）。

// DefaultSIAutoApplyQuotaPerDay is the D10 default daily auto-apply quota.
const DefaultSIAutoApplyQuotaPerDay = 5

// SINotifier delivers operator-facing notifications for the notify channel.
// Stability:evolving
type SINotifier interface {
	NotifySelfImprovement(ctx context.Context, run *SelfImprovementRun, message string) error
}

// SIApprovalSink submits a run for manual approval (high-risk / quota
// escalation). Returns the approval request ID.
// Stability:evolving
type SIApprovalSink interface {
	SubmitApproval(ctx context.Context, run *SelfImprovementRun) (approvalID string, err error)
}

// SIApplyDriver kicks off the actual apply for a run that just entered
// applying status (T4.5 hook). Implemented by SelfImprovementApplyUsecase.
// Nil → the run waits in applying for an external driver (resume worker).
// Stability:evolving
type SIApplyDriver interface {
	Apply(ctx context.Context, runID string) error
}

// SIGovernanceRouterDeps carries the router's injected dependencies.
type SIGovernanceRouterDeps struct {
	RunReader SelfImprovementRunReader
	RunWriter SelfImprovementRunWriter
	Notifier  SINotifier     // nil → notify 通道静默降级（仅日志）
	Approvals SIApprovalSink // approval 通道必需
	// ApplyDriver nil → auto/notify 迁移到 applying 后等待外部驱动。
	ApplyDriver SIApplyDriver
	// AutoApplyQuotaPerDay ≤0 → DefaultSIAutoApplyQuotaPerDay。
	AutoApplyQuotaPerDay int32
	Lg                   loggateway.Logger
}

// SIGovernanceRouter routes awaiting_governance runs to their apply channel.
type SIGovernanceRouter struct {
	runReader  SelfImprovementRunReader
	runWriter  SelfImprovementRunWriter
	notifier   SINotifier
	approvals  SIApprovalSink
	applyDrive SIApplyDriver
	dailyMax   int32
	lg         loggateway.Logger

	dailyCount atomic.Int32
	dailyReset atomic.Int64 // Unix timestamp of last daily reset
}

// NewSIGovernanceRouter wires the governance router.
func NewSIGovernanceRouter(deps SIGovernanceRouterDeps) *SIGovernanceRouter {
	dailyMax := deps.AutoApplyQuotaPerDay
	if dailyMax <= 0 {
		dailyMax = DefaultSIAutoApplyQuotaPerDay
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIGovernanceRouter{
		runReader:  deps.RunReader,
		runWriter:  deps.RunWriter,
		notifier:   deps.Notifier,
		approvals:  deps.Approvals,
		applyDrive: deps.ApplyDriver,
		dailyMax:   dailyMax,
		lg:         lg.With(loggateway.Domain("self_improve_router")),
	}
}

// Route applies the governance decision of one awaiting_governance run and
// returns the effective channel (auto escalation returns "approval").
func (r *SIGovernanceRouter) Route(ctx context.Context, runID string) (string, error) {
	if r == nil || r.runReader == nil || r.runWriter == nil {
		return "", apierror.Internal("SELF_IMPROVEMENT", "governance router not initialized")
	}
	run, err := r.runReader.GetByID(ctx, runID)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", apierror.NotFound("SELF_IMPROVEMENT", "run %s not found", runID)
	}
	if run.Status != RunStatusAwaitingGovernance {
		return "", apierror.Conflict("SELF_IMPROVEMENT", "run %s not awaiting_governance (current %s)", runID, run.Status)
	}
	if run.Governance == nil {
		return "", apierror.Internal("SELF_IMPROVEMENT", "run %s missing governance decision", runID)
	}

	switch run.Governance.Channel {
	case "auto", "notify":
		if !r.consumeAutoQuota() {
			return r.escalateToApproval(ctx, run)
		}
		return r.applyAuto(ctx, run)
	case "approval":
		if err := r.submitApproval(ctx, run); err != nil {
			return "", err
		}
		return "approval", nil
	case "reject":
		return "reject", r.reject(ctx, run)
	default:
		return "", apierror.Internal("SELF_IMPROVEMENT", "run %s unknown governance channel %q", runID, run.Governance.Channel)
	}
}

// applyAuto transitions the run to applying with the matching ApprovedBy mark;
// the notify channel additionally notifies operators (failure logged, not
// blocking).
func (r *SIGovernanceRouter) applyAuto(ctx context.Context, run *SelfImprovementRun) (string, error) {
	channel := run.Governance.Channel
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventApply)
	if err != nil {
		return "", apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --apply--> ?: %s", run.Status, err)
	}
	run.Status = to
	if channel == "notify" {
		run.ApprovedBy = "system:auto+notify"
	} else {
		run.ApprovedBy = "system:auto"
	}
	run.UpdatedAt = time.Now().UTC()
	if err := r.runWriter.Update(ctx, run, RunStatusAwaitingGovernance); err != nil {
		return "", err
	}
	r.lg.Info("self-improve run auto-applied",
		loggateway.StepID("si_router.auto_apply"),
		loggateway.Str("run_id", run.ID), loggateway.Str("channel", channel))
	if channel == "notify" && r.notifier != nil {
		msg := fmt.Sprintf("自改进补丁已自动应用（%s 风险）: run=%s kind=%s +%d/-%d",
			run.RiskLevel, run.ID, run.PatchKind, run.DiffStats.Additions, run.DiffStats.Deletions)
		if nerr := r.notifier.NotifySelfImprovement(ctx, run, msg); nerr != nil {
			r.lg.Warn("self-improve notify degraded",
				loggateway.StepID("si_router.notify"),
				loggateway.Str("run_id", run.ID), loggateway.Err(nerr))
		}
	}
	// T4.5 挂钩：路由迁移完成后驱动实际应用（git 合并/热加载）。驱动失败
	// 不回滚路由迁移——run 停留 applying，由 resume worker 重驱动。
	if r.applyDrive != nil {
		if err := r.applyDrive.Apply(ctx, run.ID); err != nil {
			return "", fmt.Errorf("drive apply: %w", err)
		}
	}
	return channel, nil
}

// escalateToApproval rewrites the decision channel to approval (quota
// exhausted, D10) and submits the run for manual approval. The run stays
// awaiting_governance.
func (r *SIGovernanceRouter) escalateToApproval(ctx context.Context, run *SelfImprovementRun) (string, error) {
	run.Governance.Channel = "approval"
	run.Governance.RuleHits = append(run.Governance.RuleHits, "auto_quota_exceeded")
	run.UpdatedAt = time.Now().UTC()
	if err := r.runWriter.Update(ctx, run, RunStatusAwaitingGovernance); err != nil {
		return "", err
	}
	r.lg.Info("self-improve auto-apply quota exhausted, escalated to approval",
		loggateway.StepID("si_router.escalate"),
		loggateway.Str("run_id", run.ID),
		loggateway.Int("daily_max", int(r.dailyMax)))
	if err := r.submitApproval(ctx, run); err != nil {
		return "", err
	}
	return "approval", nil
}

// submitApproval files a manual approval request; the run stays
// awaiting_governance until the operator approves/rejects (Phase 4 entry).
func (r *SIGovernanceRouter) submitApproval(ctx context.Context, run *SelfImprovementRun) error {
	if r.approvals == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "approval sink not wired")
	}
	if _, err := r.approvals.SubmitApproval(ctx, run); err != nil {
		return fmt.Errorf("submit approval: %w", err)
	}
	r.lg.Info("self-improve run submitted for approval",
		loggateway.StepID("si_router.approval"),
		loggateway.Str("run_id", run.ID))
	return nil
}

// reject closes the run as rejected (governance decision reject, D6 R5).
func (r *SIGovernanceRouter) reject(ctx context.Context, run *SelfImprovementRun) error {
	run.ClosedReason = siTruncateClosedReason("governance reject: " + strings.Join(run.Governance.RuleHits, ","))
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventReject)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --reject--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := r.runWriter.Update(ctx, run, RunStatusAwaitingGovernance); err != nil {
		return err
	}
	r.lg.Info("self-improve run rejected by governance",
		loggateway.StepID("si_router.reject"),
		loggateway.Str("run_id", run.ID), loggateway.Str("reason", run.ClosedReason))
	return nil
}

// consumeAutoQuota atomically takes one daily auto-apply slot (24h window,
// V2 计数模式). Returns false when the quota is exhausted.
func (r *SIGovernanceRouter) consumeAutoQuota() bool {
	now := time.Now()
	if now.Sub(time.Unix(r.dailyReset.Load(), 0)) >= 24*time.Hour {
		r.dailyCount.Store(0)
		r.dailyReset.Store(now.Unix())
	}
	for {
		cur := r.dailyCount.Load()
		if cur >= r.dailyMax {
			return false
		}
		if r.dailyCount.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}
