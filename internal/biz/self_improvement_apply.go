package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Apply orchestration (73-self-iteration-v3, design D7/D10) ───────────────
//
// SelfImprovementApplyUsecase drives runs from applying to observing:
//
//	applying ──kind 路由──► SIApplier（热加载 / 代码合并）
//	    │ 冲突（ErrSIMergeConflict）──► awaiting_governance + channel=approval（D7 转人工）
//	    │ 其他错误 ──► failed（终态）
//	    ▼ 成功
//	applied ──观察窗槽位 + 核心路径空闲──► observing（ObserveUntil = now + 24h）
//
// D10 成本控制：并发 observing ≤ maxObserving（默认 3）；design §九：命中
// 相同核心路径的补丁串行观察（观察窗指标互相污染防护）。未晋升的 run 停留
// applied 构成队列，由 Watchdog（T4.2）经 PromoteEligible 在槽位释放后晋升。

const (
	// defaultSIMaxConcurrentObserving is the D10 observing-window cap.
	defaultSIMaxConcurrentObserving = 3
	// defaultSIObserveWindow is the D7 observing-window length.
	defaultSIObserveWindow = 24 * time.Hour
	// siClosedReasonMaxLen mirrors the ent schema closed_reason MaxLen(512).
	siClosedReasonMaxLen = 512
)

// SelfImprovementApplyUsecaseDeps carries the apply usecase's injected deps.
type SelfImprovementApplyUsecaseDeps struct {
	RunReader SelfImprovementRunReader
	RunWriter SelfImprovementRunWriter
	Applier   SIApplier
	// Approvals is optional; nil → conflict escalation only rewrites the
	// governance decision (an external driver submits the approval).
	Approvals SIApprovalSink
	// MaxConcurrentObserving ≤0 → defaultSIMaxConcurrentObserving.
	MaxConcurrentObserving int
	// ObserveWindow ≤0 → defaultSIObserveWindow.
	ObserveWindow time.Duration
	// RiskRules supplies the R3 core-path globs for observe-window
	// serialization (P5 configurable rules); zero → code defaults.
	RiskRules SIRiskRules
	// Reloader is optional; nil → working-tree apply only (no in-process reload).
	Reloader SIRuntimeReloader
	Lg       loggateway.Logger
}

// SelfImprovementApplyUsecase orchestrates patch application and observing
// window admission. Safe for concurrent use.
type SelfImprovementApplyUsecase struct {
	runReader     SelfImprovementRunReader
	runWriter     SelfImprovementRunWriter
	applier       SIApplier
	approvals     SIApprovalSink
	maxObserving  int
	observeWindow time.Duration
	corePathGlobs []string
	reloader      SIRuntimeReloader
	lg            loggateway.Logger

	// promoteMu serializes the slot-check → transition critical section so
	// concurrent Apply/PromoteEligible calls cannot oversubscribe the
	// observing window.
	promoteMu sync.Mutex
}

// NewSelfImprovementApplyUsecase wires the apply orchestration usecase.
func NewSelfImprovementApplyUsecase(deps SelfImprovementApplyUsecaseDeps) (*SelfImprovementApplyUsecase, error) {
	if deps.RunReader == nil || deps.RunWriter == nil || deps.Applier == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "run reader/writer and applier are required")
	}
	maxObserving := deps.MaxConcurrentObserving
	if maxObserving <= 0 {
		maxObserving = defaultSIMaxConcurrentObserving
	}
	window := deps.ObserveWindow
	if window <= 0 {
		window = defaultSIObserveWindow
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementApplyUsecase{
		runReader:     deps.RunReader,
		runWriter:     deps.RunWriter,
		applier:       deps.Applier,
		approvals:     deps.Approvals,
		maxObserving:  maxObserving,
		observeWindow: window,
		corePathGlobs: NormalizeSIRiskRules(deps.RiskRules).CorePathGlobs,
		reloader:      deps.Reloader,
		lg:            lg.With(loggateway.Domain("self_improve_apply")),
	}, nil
}

// Apply drives one run currently in applying status (router output) through
// the applier and into the observing window. It implements the router's
// SIApplyDriver port. Apply failures are absorbed into run status transitions
// (failed / escalated); a non-nil return signals an infrastructure error.
func (uc *SelfImprovementApplyUsecase) Apply(ctx context.Context, runID string) error {
	run, err := uc.runReader.GetByID(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return apierror.NotFound("SELF_IMPROVEMENT", "run %s not found", runID)
	}
	if run.Status != RunStatusApplying {
		return apierror.Conflict("SELF_IMPROVEMENT", "run %s not applying (current %s)", runID, run.Status)
	}

	if applyErr := uc.applyByKind(ctx, run); applyErr != nil {
		if errors.Is(applyErr, ErrSIMergeConflict) {
			return uc.escalateConflict(ctx, run, applyErr)
		}
		return uc.failRun(ctx, run, applyErr)
	}

	// applying → applied.
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventApplyDone)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --apply_done--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusApplying); err != nil {
		return err
	}
	uc.lg.Info("self-improve run applied",
		loggateway.StepID("si_apply.applied"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("kind", string(run.PatchKind)),
		loggateway.Str("effective_on", siApplyEffectiveOn(run)))

	// 尝试晋升观察窗；槽位满/核心路径冲突停留 applied 由 Watchdog 晋升，非错误。
	uc.tryObserve(ctx, run)
	return nil
}

// applyByKind routes the run to the matching applier channel (D7):
// code/test → git 合并；config/prompt/docs → 热加载。
func (uc *SelfImprovementApplyUsecase) applyByKind(ctx context.Context, run *SelfImprovementRun) error {
	switch run.PatchKind {
	case PatchKindCode, PatchKindTest:
		sha, err := uc.applier.ApplyCodeMerge(ctx, run)
		if err != nil {
			return err
		}
		run.AppliedCommit = sha
		uc.annotateApply(run, siApplyChannelCode, siApplyEffectiveRestart, false)
		return nil
	case PatchKindConfig, PatchKindPrompt, PatchKindDocs:
		ref, err := uc.applier.ApplyHotReload(ctx, run)
		if err != nil {
			return err
		}
		run.RollbackPointer = ref
		reloaded := uc.tryRuntimeReload(ctx, run)
		effectiveOn := siApplyEffectiveRead
		if reloaded {
			effectiveOn = siApplyEffectiveLive
		}
		uc.annotateApply(run, siApplyChannelTree, effectiveOn, reloaded)
		return nil
	default:
		return apierror.Internal("SELF_IMPROVEMENT", "run %s unknown patch kind %q", run.ID, run.PatchKind)
	}
}

const (
	siMetaApplySemantics    = "apply_semantics"
	siApplyChannelCode      = "code_merge"
	siApplyChannelTree      = "working_tree"
	siApplyEffectiveRestart = "next_restart"
	siApplyEffectiveRead    = "next_file_read"
	siApplyEffectiveLive    = "runtime_reloaded"
)

func (uc *SelfImprovementApplyUsecase) tryRuntimeReload(ctx context.Context, run *SelfImprovementRun) bool {
	if uc.reloader == nil {
		return false
	}
	if err := uc.reloader.ReloadAfterWorkingTreeApply(ctx, run); err != nil {
		uc.lg.Warn("self-improve working-tree apply: runtime reload failed",
			loggateway.StepID("si_apply.reload"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return false
	}
	return true
}

func (uc *SelfImprovementApplyUsecase) annotateApply(run *SelfImprovementRun, channel, effectiveOn string, reloaded bool) {
	run.Metadata = siWatchMetaMerge(run.Metadata, siMetaApplySemantics, map[string]any{
		"channel":          channel,
		"effective_on":     effectiveOn,
		"runtime_reloaded": reloaded,
	})
}

func siApplyEffectiveOn(run *SelfImprovementRun) string {
	if run == nil || len(run.Metadata) == 0 {
		return ""
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(run.Metadata, &meta); err != nil {
		return ""
	}
	raw, ok := meta[siMetaApplySemantics]
	if !ok {
		return ""
	}
	var sem struct {
		EffectiveOn string `json:"effective_on"`
	}
	if err := json.Unmarshal(raw, &sem); err != nil {
		return ""
	}
	return sem.EffectiveOn
}

// escalateConflict returns a conflicted run to awaiting_governance with the
// channel rewritten to approval (D7: 冲突则转人工) and submits it for manual
// approval when the sink is wired.
func (uc *SelfImprovementApplyUsecase) escalateConflict(ctx context.Context, run *SelfImprovementRun, cause error) error {
	if run.Governance == nil {
		run.Governance = &GovernanceDecision{}
	}
	run.Governance.Channel = "approval"
	run.Governance.RuleHits = append(run.Governance.RuleHits, "merge_conflict")
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventApplyEscalate)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --apply_escalate--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusApplying); err != nil {
		return err
	}
	uc.lg.Info("self-improve merge conflict escalated to manual approval",
		loggateway.StepID("si_apply.escalate"),
		loggateway.Str("run_id", run.ID),
		loggateway.Err(cause))
	if uc.approvals != nil {
		if _, err := uc.approvals.SubmitApproval(ctx, run); err != nil {
			return fmt.Errorf("submit approval: %w", err)
		}
	}
	return nil
}

// failRun terminates the run as failed (apply-stage unrecoverable error).
func (uc *SelfImprovementApplyUsecase) failRun(ctx context.Context, run *SelfImprovementRun, cause error) error {
	run.ClosedReason = siTruncateClosedReason("apply failed: " + cause.Error())
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventError)
	if err != nil {
		return apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --error--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusApplying); err != nil {
		return err
	}
	uc.lg.Error("self-improve apply failed",
		loggateway.StepID("si_apply.failed"),
		loggateway.Str("run_id", run.ID),
		loggateway.Err(cause))
	return nil
}

// tryObserve admits a freshly applied run into the observing window when a
// slot is free and no core-path conflict exists; otherwise the run stays
// applied (Watchdog promotes later). Infra errors are logged, not returned.
func (uc *SelfImprovementApplyUsecase) tryObserve(ctx context.Context, run *SelfImprovementRun) {
	uc.promoteMu.Lock()
	defer uc.promoteMu.Unlock()
	observing, err := uc.runReader.List(ctx, RunFilter{Status: RunStatusObserving})
	if err != nil {
		uc.lg.Warn("self-improve observe admission degraded: list observing failed",
			loggateway.StepID("si_apply.observe"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	if _, _, err := uc.tryObserveLocked(ctx, run, &observing); err != nil {
		uc.lg.Warn("self-improve observe admission failed",
			loggateway.StepID("si_apply.observe"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
	}
}

// PromoteEligible admits queued applied runs into the observing window,
// oldest first, until the slot cap or a per-run core-path conflict stops
// admission. Called by the Watchdog (T4.2) each tick.
func (uc *SelfImprovementApplyUsecase) PromoteEligible(ctx context.Context) error {
	uc.promoteMu.Lock()
	defer uc.promoteMu.Unlock()
	observing, err := uc.runReader.List(ctx, RunFilter{Status: RunStatusObserving})
	if err != nil {
		return err
	}
	if len(observing) >= uc.maxObserving {
		return nil
	}
	applied, err := uc.runReader.List(ctx, RunFilter{Status: RunStatusApplied})
	if err != nil {
		return err
	}
	// data 层 List 按 created_at DESC 返回（新的在前），翻转为最老优先。
	for i, j := 0, len(applied)-1; i < j; i, j = i+1, j-1 {
		applied[i], applied[j] = applied[j], applied[i]
	}
	for i := range applied {
		_, slotFull, err := uc.tryObserveLocked(ctx, &applied[i], &observing)
		if err != nil {
			uc.lg.Warn("self-improve promote skipped",
				loggateway.StepID("si_apply.promote"),
				loggateway.Str("run_id", applied[i].ID), loggateway.Err(err))
			continue
		}
		if slotFull {
			break
		}
	}
	return nil
}

// tryObserveLocked performs the admission check and the applied→observing
// transition. The caller must hold promoteMu. observing is the current
// observing-run list; a promoted run is appended so subsequent checks in the
// same batch account for it. Returns (promoted, slotFull, error).
func (uc *SelfImprovementApplyUsecase) tryObserveLocked(ctx context.Context, run *SelfImprovementRun, observing *[]SelfImprovementRun) (bool, bool, error) {
	if len(*observing) >= uc.maxObserving {
		uc.lg.Info("self-improve observe window full, run stays applied",
			loggateway.StepID("si_apply.observe_wait"),
			loggateway.Str("run_id", run.ID),
			loggateway.Int("max_observing", uc.maxObserving))
		return false, true, nil
	}
	myAreas := SICoreAreas(run.Diff, uc.corePathGlobs)
	for _, o := range *observing {
		if SICoreAreasIntersect(myAreas, SICoreAreas(o.Diff, uc.corePathGlobs)) {
			uc.lg.Info("self-improve core path busy, run stays applied",
				loggateway.StepID("si_apply.observe_wait"),
				loggateway.Str("run_id", run.ID),
				loggateway.Str("busy_run_id", o.ID))
			return false, false, nil
		}
	}
	until := time.Now().UTC().Add(uc.observeWindow)
	run.ObserveUntil = &until
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventObserve)
	if err != nil {
		return false, false, apierror.Internal("SELF_IMPROVEMENT", "illegal run transition %s --observe--> ?: %s", run.Status, err)
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusApplied); err != nil {
		return false, false, err
	}
	*observing = append(*observing, *run)
	uc.lg.Info("self-improve run entered observing window",
		loggateway.StepID("si_apply.observe"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("observe_until", until.Format(time.RFC3339)))
	return true, false, nil
}

// siTruncateClosedReason caps ClosedReason at the ent schema MaxLen(64).
func siTruncateClosedReason(s string) string {
	if len(s) <= siClosedReasonMaxLen {
		return s
	}
	return s[:siClosedReasonMaxLen-3] + "..."
}
