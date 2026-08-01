package biz

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ── Drive orchestration (73-self-iteration-v3, Phase 4 W2b) ─────────────────
//
// SelfImprovementDriveUsecase is the full-chain driver that keeps every
// non-terminal run moving through the seven-stage loop:
//
//	detected            → Pipeline.Execute（异步，活跃集防重入）
//	diagnosing/patching/verifying（陈旧）→ recover 回 detected（崩溃孤儿 / pause 超时重驱动）
//	awaiting_governance → Router.Route（approval 通道每 run 每进程只提交一次）
//	applying            → Applier.Apply（router 未挂钩时的重驱动入口）
//	applied             → Applier.PromoteEligible（每 tick，观察窗槽位释放后晋升）
//
// 终态 / 活跃中途态 / observing 由本 usecase 跳过（observing 归 Watchdog）。
// 错误语义：ErrSIRunPaused 与 CodeConflict 静默吸收（pause 驻留 / 他入口已
// 推进）；其余错误按 run 记日志，不中断整 tick。

// defaultSIDriveStaleTimeout is the mid-pipeline stale threshold: a run whose
// UpdatedAt is older than this is treated as a crash orphan / expired pause
// and recovered to detected for re-driving.
const defaultSIDriveStaleTimeout = 30 * time.Minute

// SIPipelineExecutor executes the Meta Team pipeline for one detected run.
// Implemented by SelfImprovementPipelineUsecase.
// Stability:evolving
type SIPipelineExecutor interface {
	Execute(ctx context.Context, runID string) error
}

// SIGovernanceRoutePort routes one awaiting_governance run to its apply
// channel. Implemented by SIGovernanceRouter.
// Stability:evolving
type SIGovernanceRoutePort interface {
	Route(ctx context.Context, runID string) (string, error)
}

// SIApplyOrchestrator drives applying runs and promotes queued applied runs
// into the observing window. Implemented by SelfImprovementApplyUsecase.
// Stability:evolving
type SIApplyOrchestrator interface {
	Apply(ctx context.Context, runID string) error
	PromoteEligible(ctx context.Context) error
}

// SelfImprovementDriveDeps carries the drive usecase's injected deps.
type SelfImprovementDriveDeps struct {
	RunReader SelfImprovementRunReader
	RunWriter SelfImprovementRunWriter
	// Pipeline nil → detected runs stay put（降级）。
	Pipeline SIPipelineExecutor
	// Router nil → awaiting_governance runs stay put（降级）。
	Router SIGovernanceRoutePort
	// Applier 必需（PromoteEligible 为 D10 强制；applying 重驱动入口）。
	Applier SIApplyOrchestrator
	// StaleTimeout ≤0 → defaultSIDriveStaleTimeout。
	StaleTimeout time.Duration
	Lg           loggateway.Logger
}

// SelfImprovementDriveUsecase orchestrates the full run lifecycle across
// ticks. Safe for concurrent use.
type SelfImprovementDriveUsecase struct {
	runReader    SelfImprovementRunReader
	runWriter    SelfImprovementRunWriter
	pipeline     SIPipelineExecutor
	router       SIGovernanceRoutePort
	applier      SIApplyOrchestrator
	staleTimeout time.Duration
	lg           loggateway.Logger

	// active guards re-entry: a detected run whose pipeline goroutine is
	// still in flight must not be driven again by the next tick.
	activeMu sync.Mutex
	active   map[string]struct{}
	// routed remembers awaiting_governance runs already routed by this
	// process: the approval channel leaves the run in place, so without the
	// dedupe every tick would re-submit the approval request. Process restart
	// idempotency is the adapter's job (W6).
	routedMu sync.Mutex
	routed   map[string]struct{}
}

// NewSelfImprovementDriveUsecase wires the drive orchestration usecase.
// RunReader/RunWriter/Applier are required; Pipeline/Router may be nil
// (degraded: the matching stages are skipped).
func NewSelfImprovementDriveUsecase(deps SelfImprovementDriveDeps) (*SelfImprovementDriveUsecase, error) {
	if deps.RunReader == nil || deps.RunWriter == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "run reader/writer are required")
	}
	if deps.Applier == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "apply orchestrator is required (PromoteEligible is mandatory per D10)")
	}
	stale := deps.StaleTimeout
	if stale <= 0 {
		stale = defaultSIDriveStaleTimeout
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementDriveUsecase{
		runReader:    deps.RunReader,
		runWriter:    deps.RunWriter,
		pipeline:     deps.Pipeline,
		router:       deps.Router,
		applier:      deps.Applier,
		staleTimeout: stale,
		lg:           lg.With(loggateway.Domain("self_improve_drive")),
		active:       map[string]struct{}{},
		routed:       map[string]struct{}{},
	}, nil
}

// DriveOnce executes one drive tick over all non-terminal runs. Per-run
// failures are logged and absorbed; only infrastructure failures (list /
// promote) are returned.
func (uc *SelfImprovementDriveUsecase) DriveOnce(ctx context.Context) error {
	// D10：每 tick 先补观察窗空位（applied 队列最老优先）。
	if err := uc.applier.PromoteEligible(ctx); err != nil {
		uc.lg.Warn("self-improve drive: promote eligible degraded",
			loggateway.StepID("si_drive.promote"),
			loggateway.Err(err))
	}
	runs, err := uc.runReader.List(ctx, RunFilter{})
	if err != nil {
		return err
	}
	for i := range runs {
		uc.driveRun(ctx, &runs[i])
	}
	return nil
}

// driveRun dispatches one run by status. All per-run errors are absorbed.
func (uc *SelfImprovementDriveUsecase) driveRun(ctx context.Context, run *SelfImprovementRun) {
	switch run.Status {
	case RunStatusDetected:
		uc.driveDetected(ctx, run.ID)
	case RunStatusDiagnosing, RunStatusPatching, RunStatusVerifying:
		uc.recoverIfStale(ctx, run)
	case RunStatusAwaitingGovernance:
		uc.routeOnce(ctx, run.ID)
	case RunStatusApplying:
		uc.driveApplying(ctx, run.ID)
	default:
		// applied（等 PromoteEligible）/ observing（归 Watchdog）/ 终态：跳过。
	}
}

// driveDetected kicks off the pipeline asynchronously, guarded by the active
// set so a slow pipeline is never re-entered by later ticks.
func (uc *SelfImprovementDriveUsecase) driveDetected(ctx context.Context, runID string) {
	if uc.pipeline == nil {
		return
	}
	uc.activeMu.Lock()
	if _, ok := uc.active[runID]; ok {
		uc.activeMu.Unlock()
		return
	}
	uc.active[runID] = struct{}{}
	uc.activeMu.Unlock()
	safego.Go(ctx, "self_improve.drive_pipeline", func() {
		defer func() {
			uc.activeMu.Lock()
			delete(uc.active, runID)
			uc.activeMu.Unlock()
		}()
		uc.lg.Info("self-improve pipeline started",
			loggateway.StepID("si_drive.pipeline"),
			loggateway.Str("run_id", runID))
		err := uc.pipeline.Execute(ctx, runID)
		switch {
		case err == nil:
			uc.lg.Info("self-improve pipeline exited",
				loggateway.StepID("si_drive.pipeline"),
				loggateway.Str("run_id", runID))
		case errors.Is(err, ErrSIRunPaused):
			uc.lg.Info("self-improve run paused by user, stays for resume",
				loggateway.StepID("si_drive.paused"),
				loggateway.Str("run_id", runID))
		default:
			uc.lg.Error("self-improve pipeline failed",
				loggateway.StepID("si_drive.pipeline_error"),
				loggateway.Str("run_id", runID),
				loggateway.Err(err))
		}
	})
}

// recoverIfStale resets a stale mid-pipeline run (crash orphan / expired
// pause) back to detected so the next tick re-drives it. Fresh mid-pipeline
// runs are left alone (in flight or recently paused).
func (uc *SelfImprovementDriveUsecase) recoverIfStale(ctx context.Context, run *SelfImprovementRun) {
	if time.Since(run.UpdatedAt) < uc.staleTimeout {
		return
	}
	from := run.Status
	to, err := NewSelfImprovementRunStateMachine().Transition(from, RunEventRecover)
	if err != nil {
		uc.lg.Error("self-improve recover: illegal transition",
			loggateway.StepID("si_drive.recover"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("status", string(from)),
			loggateway.Err(err))
		return
	}
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, from); err != nil {
		if apierror.IsCode(err, apierror.CodeConflict) {
			return // 他入口已推进，静默。
		}
		uc.lg.Warn("self-improve recover persist failed",
			loggateway.StepID("si_drive.recover"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve stale run recovered to detected",
		loggateway.StepID("si_drive.recover"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("from", string(from)))
}

// routeOnce routes an awaiting_governance run at most once per process. The
// approval channel leaves the run in place, so re-routing would re-submit
// the approval request every tick.
func (uc *SelfImprovementDriveUsecase) routeOnce(ctx context.Context, runID string) {
	if uc.router == nil {
		return
	}
	uc.routedMu.Lock()
	if _, ok := uc.routed[runID]; ok {
		uc.routedMu.Unlock()
		return
	}
	uc.routedMu.Unlock()
	channel, err := uc.router.Route(ctx, runID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeConflict) {
			return // 他入口已推进，静默。
		}
		uc.lg.Warn("self-improve route failed",
			loggateway.StepID("si_drive.route"),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
		return
	}
	uc.routedMu.Lock()
	uc.routed[runID] = struct{}{}
	uc.routedMu.Unlock()
	uc.lg.Info("self-improve run routed",
		loggateway.StepID("si_drive.route"),
		loggateway.Str("run_id", runID),
		loggateway.Str("channel", channel))
}

// driveApplying re-drives a run left in applying (e.g. router apply-driver
// hook absent or process restart between transition and apply).
func (uc *SelfImprovementDriveUsecase) driveApplying(ctx context.Context, runID string) {
	if err := uc.applier.Apply(ctx, runID); err != nil {
		if apierror.IsCode(err, apierror.CodeConflict) {
			return // 他入口已推进，静默。
		}
		uc.lg.Warn("self-improve apply drive failed",
			loggateway.StepID("si_drive.apply"),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
}
