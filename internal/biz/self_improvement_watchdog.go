package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Watchdog (73-self-iteration-v3, design D7 / §5 self_improve_watchdog) ───
//
// SelfImprovementWatchdogUsecase scans observing runs each tick:
//
//	首次见到（无基线）→ 采集应用前后对比基线（当前 1h 滑窗）写入 run.Metadata
//	观察窗未到期      → 跳过
//	到期              → 采集 after 快照，对比基线：
//	                    错误率 >before×errFactor 或 P95 >before×p95Factor → 自动回滚
//	                    → rolled_back + 管理员通知；否则 → closed（确认有效）
//
// 零基线防护：before=0（观察前低/无流量）时改用绝对地板（错误率 10%、
// P95 2000ms），避免"任何非零即无穷倍"的误判。回滚失败 / 指标失败 / CAS
// 冲突逐 run 吸收（run 停留 observing 下 tick 重试），不中断整 tick。

const (
	// siMetaObserveBaseline is the run.Metadata key of the pre-apply metrics
	// baseline (1h sliding window captured at first watchdog sight).
	siMetaObserveBaseline = "observe_baseline"
	// siMetaObserveAfter is the run.Metadata key of the post-apply metrics
	// snapshot captured at observe-window expiry (Outcome worker attribution
	// source, D8).
	siMetaObserveAfter = "observe_after"
	// defaultSIWatchMetricsWindow is the D7 comparison window (1h).
	defaultSIWatchMetricsWindow = time.Hour
	// defaultSIObserveErrFactor / defaultSIObserveP95Factor mirror design
	// D7 (错误率 +50% / P95 +30%).
	defaultSIObserveErrFactor = 1.5
	defaultSIObserveP95Factor = 1.3
	// siWatchErrorRateFloor / siWatchP95FloorMS are the absolute regression
	// floors used when the baseline is zero (no/low pre-apply traffic).
	siWatchErrorRateFloor = 0.10
	siWatchP95FloorMS     = 2000.0
)

// SelfImprovementWatchdogDeps carries the watchdog usecase's injected deps.
type SelfImprovementWatchdogDeps struct {
	RunReader SelfImprovementRunReader
	RunWriter SelfImprovementRunWriter
	Metrics   SIMetricsReader
	Applier   SIApplier
	// Notifier nil → 回滚通知降级为仅日志。
	Notifier SINotifier
	// ErrorRateFactor ≤0 → defaultSIObserveErrFactor。
	ErrorRateFactor float64
	// P95Factor ≤0 → defaultSIObserveP95Factor。
	P95Factor float64
	// MetricsWindow ≤0 → defaultSIWatchMetricsWindow。
	MetricsWindow time.Duration
	Lg            loggateway.Logger
}

// SelfImprovementWatchdogUsecase evaluates observing runs at window expiry.
type SelfImprovementWatchdogUsecase struct {
	runReader SelfImprovementRunReader
	runWriter SelfImprovementRunWriter
	metrics   SIMetricsReader
	applier   SIApplier
	notifier  SINotifier
	errFactor float64
	p95Factor float64
	window    time.Duration
	lg        loggateway.Logger
}

// NewSelfImprovementWatchdogUsecase wires the watchdog usecase.
// RunReader/RunWriter/Metrics/Applier are required; Notifier may be nil.
func NewSelfImprovementWatchdogUsecase(deps SelfImprovementWatchdogDeps) (*SelfImprovementWatchdogUsecase, error) {
	if deps.RunReader == nil || deps.RunWriter == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "run reader/writer are required")
	}
	if deps.Metrics == nil || deps.Applier == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "metrics reader and applier are required")
	}
	errFactor := deps.ErrorRateFactor
	if errFactor <= 0 {
		errFactor = defaultSIObserveErrFactor
	}
	p95Factor := deps.P95Factor
	if p95Factor <= 0 {
		p95Factor = defaultSIObserveP95Factor
	}
	window := deps.MetricsWindow
	if window <= 0 {
		window = defaultSIWatchMetricsWindow
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementWatchdogUsecase{
		runReader: deps.RunReader, runWriter: deps.RunWriter,
		metrics: deps.Metrics, applier: deps.Applier, notifier: deps.Notifier,
		errFactor: errFactor, p95Factor: p95Factor, window: window,
		lg: lg.With(loggateway.Domain("self_improve_watchdog")),
	}, nil
}

// ScanOnce executes one watchdog tick over all observing runs. Per-run
// failures are absorbed; only the list failure is returned.
func (uc *SelfImprovementWatchdogUsecase) ScanOnce(ctx context.Context) error {
	runs, err := uc.runReader.List(ctx, RunFilter{Status: RunStatusObserving})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range runs {
		uc.scanRun(ctx, &runs[i], now)
	}
	return nil
}

// scanRun processes one observing run: baseline capture → due evaluation.
func (uc *SelfImprovementWatchdogUsecase) scanRun(ctx context.Context, run *SelfImprovementRun, now time.Time) {
	baseline := siWatchBaselineFromMeta(run.Metadata)
	if baseline == nil {
		uc.captureBaseline(ctx, run)
		return
	}
	if run.ObserveUntil == nil || run.ObserveUntil.After(now) {
		return // 观察窗未到期。
	}
	after, err := uc.metrics.Snapshot(ctx, uc.window)
	if err != nil {
		uc.lg.Warn("self-improve watchdog: after-snapshot failed, run stays observing",
			loggateway.StepID("si_watchdog.snapshot"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	uc.storeAfterSnapshot(ctx, run, after)
	if reason, regressed := siWatchRegression(baseline, after, uc.errFactor, uc.p95Factor); regressed {
		uc.rollbackRun(ctx, run, reason)
		return
	}
	uc.closeRun(ctx, run)
}

// captureBaseline stores the current 1h window as the pre-apply baseline and
// leaves the run observing (evaluation starts once a baseline exists).
func (uc *SelfImprovementWatchdogUsecase) captureBaseline(ctx context.Context, run *SelfImprovementRun) {
	snap, err := uc.metrics.Snapshot(ctx, uc.window)
	if err != nil {
		uc.lg.Warn("self-improve watchdog: baseline snapshot failed",
			loggateway.StepID("si_watchdog.baseline"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	meta := siWatchMetaMerge(run.Metadata, siMetaObserveBaseline, snap)
	run.Metadata = meta
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusObserving); err != nil {
		uc.lg.Warn("self-improve watchdog: baseline persist failed",
			loggateway.StepID("si_watchdog.baseline"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve watchdog: baseline captured",
		loggateway.StepID("si_watchdog.baseline"),
		loggateway.Str("run_id", run.ID))
}

// storeAfterSnapshot best-effort persists the expiry snapshot for the
// Outcome worker's PatchOutcome attribution (D8).
func (uc *SelfImprovementWatchdogUsecase) storeAfterSnapshot(ctx context.Context, run *SelfImprovementRun, after *MetricsSnapshot) {
	run.Metadata = siWatchMetaMerge(run.Metadata, siMetaObserveAfter, after)
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, RunStatusObserving); err != nil {
		uc.lg.Warn("self-improve watchdog: after-snapshot persist degraded",
			loggateway.StepID("si_watchdog.snapshot"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
	}
}

// rollbackRun auto-reverts a regressed run (D7) and notifies operators.
func (uc *SelfImprovementWatchdogUsecase) rollbackRun(ctx context.Context, run *SelfImprovementRun, reason string) {
	if err := uc.applier.Rollback(ctx, run, reason); err != nil {
		uc.lg.Error("self-improve watchdog: auto rollback failed, retry next tick",
			loggateway.StepID("si_watchdog.rollback"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	run.ClosedReason = siTruncateClosedReason("auto rollback: " + reason)
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventRollback)
	if err != nil {
		uc.lg.Error("self-improve watchdog: illegal rollback transition",
			loggateway.StepID("si_watchdog.rollback"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	from := run.Status
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, from); err != nil {
		uc.lg.Warn("self-improve watchdog: rollback persist conflict, skipped",
			loggateway.StepID("si_watchdog.rollback"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve run auto-rolled-back",
		loggateway.StepID("si_watchdog.rollback"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("reason", reason))
	if uc.notifier != nil {
		msg := fmt.Sprintf("自改进补丁观察窗指标退化，已自动回滚: run=%s reason=%s", run.ID, reason)
		if nerr := uc.notifier.NotifySelfImprovement(ctx, run, msg); nerr != nil {
			uc.lg.Warn("self-improve watchdog: notify degraded",
				loggateway.StepID("si_watchdog.notify"),
				loggateway.Str("run_id", run.ID), loggateway.Err(nerr))
		}
	}
}

// closeRun terminates a due run as closed (观察期满无退化，确认有效).
func (uc *SelfImprovementWatchdogUsecase) closeRun(ctx context.Context, run *SelfImprovementRun) {
	run.ClosedReason = "observe window passed"
	to, err := NewSelfImprovementRunStateMachine().Transition(run.Status, RunEventClose)
	if err != nil {
		uc.lg.Error("self-improve watchdog: illegal close transition",
			loggateway.StepID("si_watchdog.close"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	from := run.Status
	run.Status = to
	run.UpdatedAt = time.Now().UTC()
	if err := uc.runWriter.Update(ctx, run, from); err != nil {
		uc.lg.Warn("self-improve watchdog: close persist conflict, skipped",
			loggateway.StepID("si_watchdog.close"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve run closed after observe window",
		loggateway.StepID("si_watchdog.close"),
		loggateway.Str("run_id", run.ID))
}

// siWatchRegression compares after vs before with the configured factors and
// zero-baseline floors. Returns (reason, regressed).
func siWatchRegression(before, after *MetricsSnapshot, errFactor, p95Factor float64) (string, bool) {
	errThreshold := before.ErrorRate * errFactor
	if before.ErrorRate <= 0 {
		errThreshold = siWatchErrorRateFloor
	}
	if after.ErrorRate > errThreshold {
		return fmt.Sprintf("error_rate %.3f > baseline %.3f", after.ErrorRate, before.ErrorRate), true
	}
	p95Threshold := before.P95MS * p95Factor
	if before.P95MS <= 0 {
		p95Threshold = siWatchP95FloorMS
	}
	if after.P95MS > p95Threshold {
		return fmt.Sprintf("p95 %.0fms > baseline %.0fms", after.P95MS, before.P95MS), true
	}
	return "", false
}

// siWatchBaselineFromMeta extracts the baseline snapshot from run.Metadata.
func siWatchBaselineFromMeta(raw json.RawMessage) *MetricsSnapshot {
	if len(raw) == 0 {
		return nil
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	sub, ok := meta[siMetaObserveBaseline]
	if !ok {
		return nil
	}
	var snap MetricsSnapshot
	if err := json.Unmarshal(sub, &snap); err != nil {
		return nil
	}
	return &snap
}

// siWatchMetaMerge returns run.Metadata with key replaced by value, keeping
// any other keys intact.
func siWatchMetaMerge(raw json.RawMessage, key string, value any) json.RawMessage {
	meta := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &meta) // 损坏 metadata 不阻断：覆盖重建。
	}
	if v, err := json.Marshal(value); err == nil {
		meta[key] = v
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return raw
	}
	return out
}
