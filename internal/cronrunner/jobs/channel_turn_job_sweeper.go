package jobs

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	turnJobSweeperDefaultInterval = 2 * time.Minute
	turnJobQueuedTimeoutDefault   = 30 * time.Minute
	turnJobAsyncRecoveryMinAge    = 2 * time.Minute
	turnJobAsyncMaxAge            = 24 * time.Hour
	turnJobSweeperBatchLimit      = 100
)

// ExecutionStatusReader queries async execution status for recovery (narrow port).
// *biz.GraphUsecase satisfies this interface.
type ExecutionStatusReader interface {
	GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error)
}

// ChannelTurnJobSweeper periodically recovers stuck ChannelTurnJob records:
//  1. Queued jobs that exceeded the queue timeout → Timeout (prevents permanent stuck queue).
//  2. AsyncQueued jobs whose watch goroutine was lost (process restart) → recovered by
//     checking the actual async target (graph/cron) status and transitioning accordingly.
//  3. Startup reconcile (once, before the first sweep): Accepted/Running jobs whose
//     updated_at predates this process start belong to a dead process — their in-memory
//     execution goroutine is gone and no watcher will ever finish them → Failed.
//
// This sweeper is the safety net for P0 issues #8 (Queued timeout) and #9 (AsyncQueued recovery).
// It can be disabled via CHANNEL_TURN_JOB_SWEEPER_DISABLED env var.
type ChannelTurnJobSweeper struct {
	interval      time.Duration
	queuedTimeout time.Duration
	jobs          biz.ChannelTurnJobRepo
	graphExec     ExecutionStatusReader
	cron          biz.CronTriggerGateway
	lg            loggateway.Logger
	// processStart is captured at construction (wire build, before the ready gate
	// opens traffic). The startup reconcile only touches jobs with
	// updated_at < processStart, so jobs created by the current process are never
	// matched — this makes the reconcile safe even though Start runs after the
	// ready gate.
	// TODO(multi-instance): with more than one instance this must become
	// claim/lease-based; a restarting instance would otherwise fail peers' jobs.
	processStart time.Time
}

// NewChannelTurnJobSweeper creates a sweeper for stuck ChannelTurnJob recovery.
// graphExec may be nil if graph recovery is not needed (cron-only deployments).
// cron may be nil if cron recovery is not needed.
func NewChannelTurnJobSweeper(
	interval time.Duration,
	queuedTimeout time.Duration,
	jobs biz.ChannelTurnJobRepo,
	graphExec ExecutionStatusReader,
	cron biz.CronTriggerGateway,
	lg loggateway.Logger,
) *ChannelTurnJobSweeper {
	if interval <= 0 {
		interval = turnJobSweeperDefaultInterval
	}
	if queuedTimeout <= 0 {
		queuedTimeout = turnJobQueuedTimeoutDefault
	}
	return &ChannelTurnJobSweeper{
		interval:      interval,
		queuedTimeout: queuedTimeout,
		jobs:          jobs,
		graphExec:     graphExec,
		cron:          cron,
		lg:            lg,
		processStart:  time.Now().UTC(),
	}
}

func (w *ChannelTurnJobSweeper) Start(ctx context.Context) {
	if w == nil || w.jobs == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.sweepStartupInterrupted(ctx)
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// sweepStartupInterrupted fails Accepted/Running jobs whose updated_at predates
// this process start (startup reconcile 兜底). Their execution goroutine died
// with the previous process, so without this pass they would stay non-terminal
// forever — the periodic sweeper only scans Queued/AsyncQueued.
//
// The processStart cutoff makes this safe to run after the ready gate: jobs
// created by the current process always have updated_at >= processStart
// (the sweeper is constructed during wire build, before traffic is served).
// Runs once per process; errors only degrade this pass, never block startup.
func (w *ChannelTurnJobSweeper) sweepStartupInterrupted(ctx context.Context) {
	cutoff := w.processStart.Format(time.RFC3339Nano)
	for _, status := range []string{biz.ChannelTurnJobStatusAccepted, biz.ChannelTurnJobStatusRunning} {
		jobs, err := w.jobs.ListStaleByStatus(ctx, status, cutoff, turnJobSweeperBatchLimit)
		if err != nil {
			w.lg.Warn("TurnJob sweeper: 启动对账扫描失败",
				loggateway.StepID("channel.turn_job_sweeper.startup_scan_failed"),
				loggateway.Str("status", status),
				loggateway.Err(err),
			)
			continue
		}
		var failed int
		for _, job := range jobs {
			errMsg := "turn execution interrupted by process restart (startup reconcile)"
			if err := w.jobs.UpdateStatus(ctx, job.ID, biz.ChannelTurnJobStatusFailed, errMsg, "", ""); err != nil {
				w.lg.Warn("TurnJob sweeper: 启动对账转换失败",
					loggateway.StepID("channel.turn_job_sweeper.startup_transition_failed"),
					loggateway.Str("job_id", job.ID),
					loggateway.Err(err),
				)
				continue
			}
			failed++
		}
		if failed > 0 {
			w.lg.Info("TurnJob sweeper: 启动对账完成",
				loggateway.StepID("channel.turn_job_sweeper.startup_done"),
				loggateway.Str("status", status),
				loggateway.Int("failed", failed),
			)
		}
	}
}

func (w *ChannelTurnJobSweeper) runOnce(ctx context.Context) {
	safego.Go(ctx, "channel.turn_job_sweeper", func() {
		w.sweepQueuedTimeouts(ctx)
		w.sweepAsyncQueuedRecovery(ctx)
	})
}

// RunOnceExposed runs a single sweep cycle synchronously (for testing).
func (w *ChannelTurnJobSweeper) RunOnceExposed(ctx context.Context) {
	w.sweepQueuedTimeouts(ctx)
	w.sweepAsyncQueuedRecovery(ctx)
}

// sweepQueuedTimeouts transitions queued jobs that exceeded the queue timeout to Timeout.
// This handles the case where the active turn is stuck and queued messages would wait forever.
func (w *ChannelTurnJobSweeper) sweepQueuedTimeouts(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-w.queuedTimeout).Format(time.RFC3339Nano)
	jobs, err := w.jobs.ListStaleByStatus(ctx, biz.ChannelTurnJobStatusQueued, cutoff, turnJobSweeperBatchLimit)
	if err != nil {
		w.lg.Warn("TurnJob sweeper: queued 超时扫描失败",
			loggateway.StepID("channel.turn_job_sweeper.queued_scan_failed"),
			loggateway.Err(err),
		)
		return
	}
	var recovered int
	for _, job := range jobs {
		errMsg := "queued turn job timed out (sweeper recovery)"
		if err := w.jobs.UpdateStatus(ctx, job.ID, biz.ChannelTurnJobStatusTimeout, errMsg, "", ""); err != nil {
			w.lg.Warn("TurnJob sweeper: queued 超时转换失败",
				loggateway.StepID("channel.turn_job_sweeper.queued_timeout_failed"),
				loggateway.Str("job_id", job.ID),
				loggateway.Err(err),
			)
			continue
		}
		recovered++
	}
	if recovered > 0 {
		w.lg.Info("TurnJob sweeper: queued 超时清理完成",
			loggateway.StepID("channel.turn_job_sweeper.queued_done"),
			loggateway.Int("recovered", recovered),
		)
	}
}

// sweepAsyncQueuedRecovery recovers AsyncQueued jobs whose watch goroutine was lost
// (typically due to process restart). It checks the actual async target status:
//   - Graph: query graph_executions status
//   - Cron: query cron_task_runs status
//
// If the target is terminal, the job is transitioned to the matching terminal state.
// If the target is still running, the job's updated_at is refreshed to prevent repeated scans.
// Jobs older than turnJobAsyncMaxAge are force-timed-out regardless of target status.
func (w *ChannelTurnJobSweeper) sweepAsyncQueuedRecovery(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-turnJobAsyncRecoveryMinAge).Format(time.RFC3339Nano)
	jobs, err := w.jobs.ListStaleByStatus(ctx, biz.ChannelTurnJobStatusAsyncQueued, cutoff, turnJobSweeperBatchLimit)
	if err != nil {
		w.lg.Warn("TurnJob sweeper: async_queued 恢复扫描失败",
			loggateway.StepID("channel.turn_job_sweeper.async_scan_failed"),
			loggateway.Err(err),
		)
		return
	}
	var recovered int
	for _, job := range jobs {
		w.recoverAsyncJob(ctx, job)
		recovered++
	}
	if recovered > 0 {
		w.lg.Info("TurnJob sweeper: async_queued 恢复扫描完成",
			loggateway.StepID("channel.turn_job_sweeper.async_done"),
			loggateway.Int("scanned", recovered),
		)
	}
}

func (w *ChannelTurnJobSweeper) recoverAsyncJob(ctx context.Context, job biz.ChannelTurnJob) {
	// Force-timeout jobs that exceeded the global async max age.
	if age, err := time.Parse(time.RFC3339, job.UpdatedAt); err == nil {
		if time.Since(age) > turnJobAsyncMaxAge {
			w.forceTimeoutAsyncJob(ctx, job, "async job exceeded max age (sweeper recovery)")
			return
		}
	}

	targetType := strings.TrimSpace(job.AsyncTargetType)
	targetID := strings.TrimSpace(job.AsyncTargetID)
	if targetID == "" {
		w.forceTimeoutAsyncJob(ctx, job, "async job has no target ID (sweeper recovery)")
		return
	}

	switch targetType {
	case "graph", "team_graph":
		w.recoverGraphAsyncJob(ctx, job, targetID)
	case "cron":
		w.recoverCronAsyncJob(ctx, job, targetID)
	default:
		w.forceTimeoutAsyncJob(ctx, job, "async job has unknown target type: "+targetType)
	}
}

func (w *ChannelTurnJobSweeper) recoverGraphAsyncJob(ctx context.Context, job biz.ChannelTurnJob, execID string) {
	if w.graphExec == nil {
		return
	}
	exec, err := w.graphExec.GetExecution(ctx, execID)
	if err != nil {
		w.lg.Warn("TurnJob sweeper: graph execution 查询失败",
			loggateway.StepID("channel.turn_job_sweeper.graph_lookup_failed"),
			loggateway.Str("job_id", job.ID),
			loggateway.Str("exec_id", execID),
			loggateway.Err(err),
		)
		return
	}
	if exec == nil {
		w.forceTimeoutAsyncJob(ctx, job, "graph execution not found (sweeper recovery)")
		return
	}
	status := strings.TrimSpace(exec.Status)
	switch biz.ParseGraphExecutionState(status) {
	case biz.GraphExecCompleted:
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusCompleted, "", "graph completed (sweeper recovery)")
	case biz.GraphExecFailed:
		errMsg := strings.TrimSpace(exec.ErrorMessage)
		if errMsg == "" {
			errMsg = "graph execution failed (sweeper recovery)"
		}
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusFailed, errMsg, "")
	case biz.GraphExecCancelled:
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusCancelled, "graph cancelled (sweeper recovery)", "")
	default:
		// Still running — refresh updated_at to prevent repeated scans.
		w.touchAsyncJob(ctx, job)
	}
}

func (w *ChannelTurnJobSweeper) recoverCronAsyncJob(ctx context.Context, job biz.ChannelTurnJob, runID string) {
	if w.cron == nil {
		return
	}
	run, err := w.cron.GetTaskRun(ctx, runID)
	if err != nil {
		if errors.Is(err, biz.ErrCronNotFound) {
			w.forceTimeoutAsyncJob(ctx, job, "cron run not found (sweeper recovery)")
		} else {
			w.lg.Warn("TurnJob sweeper: cron run 查询失败",
				loggateway.StepID("channel.turn_job_sweeper.cron_lookup_failed"),
				loggateway.Str("job_id", job.ID),
				loggateway.Str("run_id", runID),
				loggateway.Err(err),
			)
		}
		return
	}
	switch strings.ToLower(strings.TrimSpace(run.Status)) {
	case "success":
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusCompleted, "", "cron task succeeded (sweeper recovery)")
	case "failure", "failed":
		errMsg := strings.TrimSpace(run.ErrorMessage)
		if errMsg == "" {
			errMsg = "cron task failed (sweeper recovery)"
		}
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusFailed, errMsg, "")
	case "skipped":
		w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusCompleted, "", "cron task skipped (sweeper recovery)")
	default:
		// Still running — refresh updated_at to prevent repeated scans.
		w.touchAsyncJob(ctx, job)
	}
}

// transitionAsyncJob uses UpdateStatus directly because the sweeper operates on jobs
// that may have been stuck since before the current process started — the state machine
// validation in TransitionByEvent would require loading the job first, but we already have it.
// The state machine consistency is guaranteed by the transition rules we follow:
//   - AsyncQueued → Completed (via async completion)
//   - AsyncQueued → Failed (via async_fail)
//   - AsyncQueued → Cancelled (via async_cancel)
//   - AsyncQueued → Timeout (via timeout)
func (w *ChannelTurnJobSweeper) transitionAsyncJob(ctx context.Context, job biz.ChannelTurnJob, targetStatus, errMsg, preview string) {
	if err := w.jobs.UpdateStatus(ctx, job.ID, targetStatus, errMsg, "", preview); err != nil {
		w.lg.Warn("TurnJob sweeper: async job 状态转换失败",
			loggateway.StepID("channel.turn_job_sweeper.async_transition_failed"),
			loggateway.Str("job_id", job.ID),
			loggateway.Str("target_status", targetStatus),
			loggateway.Err(err),
		)
		return
	}
	w.lg.Info("TurnJob sweeper: async job 已恢复",
		loggateway.StepID("channel.turn_job_sweeper.async_recovered"),
		loggateway.Str("job_id", job.ID),
		loggateway.Str("target_status", targetStatus),
		loggateway.Str("target_type", job.AsyncTargetType),
		loggateway.Str("target_id", job.AsyncTargetID),
	)
}

func (w *ChannelTurnJobSweeper) forceTimeoutAsyncJob(ctx context.Context, job biz.ChannelTurnJob, reason string) {
	w.transitionAsyncJob(ctx, job, biz.ChannelTurnJobStatusTimeout, reason, "")
}

// touchAsyncJob refreshes updated_at so the sweeper doesn't repeatedly scan
// jobs whose async target is still legitimately running.
func (w *ChannelTurnJobSweeper) touchAsyncJob(ctx context.Context, job biz.ChannelTurnJob) {
	_ = w.jobs.UpdateStatus(ctx, job.ID, biz.ChannelTurnJobStatusAsyncQueued, "", "", "")
}

func ChannelTurnJobSweeperDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHANNEL_TURN_JOB_SWEEPER_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
