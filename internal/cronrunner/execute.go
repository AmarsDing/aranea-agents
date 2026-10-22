package cronrunner

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type runOutcome struct {
	result cronDispatchResult
	status string
	errMsg string
	// foldBase: 本次跳过/失败被折叠进的历史行已代表的执行次数（0 = 未折叠，新起一行）。
	// finalizeRun 将其+1 写入 output.fold_count。
	foldBase int
}

func (r *Runner) lockTask(taskID string) func() {
	// Named registry (CRON-01): LoadOrStore is atomic for the same taskID.
	return r.taskMu.lock(taskID)
}

func (r *Runner) insertPendingRun(ctx context.Context, taskID, trigger string, now time.Time) (runID, started string, ok bool) {
	runID = uuid.NewString()
	started = now.UTC().Format(time.RFC3339)
	outputPending := mustMarshalJSON(map[string]any{"trigger": trigger})
	err := r.deps.Cron.InsertCronTaskRun(ctx, biz.CronTaskRunInput{
		ID:         runID,
		TaskID:     taskID,
		Status:     "pending",
		StartedAt:  started,
		OutputJSON: outputPending,
		CreatedAt:  started,
	})
	return runID, started, err == nil
}

func (r *Runner) runDispatch(ctx context.Context, task biz.CronTask, cfg cronTaskConfig) runOutcome {
	startedTime := time.Now()
	flow := r.flowEmitter(ctx, "", "")
	if flow != nil {
		flow.LogStart("cron.job.dispatch", "定时任务分发",
			event.P("job_id", task.ID),
			event.P("job_type", cronTargetType(cfg)))
	}
	result, execErr := r.dispatchWithRetry(ctx, task, cfg)
	elapsed := time.Since(startedTime)
	cronJobDuration.WithLabelValues(task.ID).Observe(elapsed.Seconds())

	if execErr == nil {
		if flow != nil {
			flow.LogDone("cron.job.dispatch", "定时任务分发完成",
				event.P("job_id", task.ID),
				event.P("session_id", result.SessionID))
			flow.LogDone("cron.job.execute", "定时任务执行完成",
				event.P("job_id", task.ID),
				event.P("session_id", result.SessionID),
				event.P("elapsed_ms", elapsed.Milliseconds()),
				event.P("status", "success"))
		}
		return runOutcome{result: result, status: "success"}
	}
	if isSessionBusyErr(execErr) {
		// 跳过分支的流程日志已在 sessionBusyErr 发射，此处不重复。
		return runOutcome{result: result, status: "skipped", errMsg: execErr.Error()}
	}
	if flow != nil {
		flow.LogError("cron.job.execute", "定时任务执行失败",
			event.P("job_id", task.ID),
			event.P("elapsed_ms", elapsed.Milliseconds()),
			event.P("error", execErr.Error()))
	}
	return runOutcome{result: result, status: "failure", errMsg: execErr.Error()}
}

func (r *Runner) finalizeRun(
	ctx context.Context,
	runID string,
	task biz.CronTask,
	cfg cronTaskConfig,
	meta cronTaskMetadata,
	started, trigger string,
	outcome runOutcome,
) {
	finishedAt := time.Now().UTC()
	finished := finishedAt.Format(time.RFC3339)
	output := map[string]any{
		"trigger":          trigger,
		"target_type":      cronTargetType(cfg),
		"session_id":       outcome.result.SessionID,
		"user_message_id":  outcome.result.UserMessageID,
		"agent_message_id": outcome.result.AgentMessageID,
		"run_id":           outcome.result.SessionID,
	}
	if outcome.foldBase > 0 {
		output["fold_count"] = outcome.foldBase + 1
	}
	outJSON := mustMarshalJSON(output)
	if err := r.deps.Cron.UpdateCronTaskRun(ctx, runID, outcome.status, finished, outJSON, outcome.errMsg); err != nil {
		r.lg.Warn("update cron_task_run failed", loggateway.Str("task_id", task.ID), loggateway.Str("run_id", runID), loggateway.Err(err))
	}
	cronJobRunsTotal.WithLabelValues(task.ID, outcome.status).Inc()

	manual := trigger == "manual"
	meta.RunCount++
	meta.LastRunAt = finished
	meta.LastRunStatus = outcome.status

	switch outcome.status {
	case "success":
		meta.SuccessCount++
		meta.LastError = ""
		meta.SkipCount = 0
		// Any successful execution (manual or scheduled) proves the task is
		// healthy, so reset the consecutive failure counter.
		meta.FailureCount = 0
	case "skipped", cronRunStatusSkippedMissed:
		// 跳过一族（会话忙/错过补跑窗口/配置不可求值）不推进死信计数：
		// 它们不是「执行失败」，独立累计 skip_count 供观测（H11/orca recordUnevaluable*）。
		meta.LastError = outcome.errMsg
		meta.SkipCount++
	case "failure":
		meta.LastError = outcome.errMsg
		meta.RecentFailure = append([]cronFailureSummary{{
			StartedAt:    started,
			ErrorMessage: outcome.errMsg,
		}}, meta.RecentFailure...)
		if len(meta.RecentFailure) > 5 {
			meta.RecentFailure = meta.RecentFailure[:5]
		}
		if !manual {
			meta.FailureCount++
			if meta.FailureCount >= maxDeadFailures && task.Status != "dead" {
				task.Status = "dead"
				task.Enabled = false
				cronJobDeadTotal.WithLabelValues(task.ID).Inc()
				r.lg.Error("定时任务进入死信",
					loggateway.StepID("cron.job_dead"),
					loggateway.Str("job_id", task.ID),
					loggateway.Str("task_key", task.TaskKey),
					loggateway.Int("failure_count", meta.FailureCount))
				if flow := r.flowEmitter(ctx, outcome.result.SessionID, runID); flow != nil {
					flow.LogError("system.cron.job_dead", "定时任务进入死信",
						event.P("job_id", task.ID),
						event.P("task_key", task.TaskKey),
						event.P("failure_count", meta.FailureCount))
				}
				r.publishDeadLetterEvent(ctx, task)
			}
		}
	}

	if !manual {
		if cfg.ScheduleType == "once" {
			task.Enabled = false
			if task.Status != "dead" {
				task.Status = "paused"
			}
			meta.NextRunAt = ""
		} else if next, err := nextCronRunAfter(cfg, finishedAt); err == nil {
			meta.NextRunAt = next.Format(time.RFC3339)
		}
	}

	rawMeta, err := json.Marshal(meta)
	if err != nil {
		r.lg.Warn("marshal metadata failed", loggateway.Str("task_id", task.ID), loggateway.Err(err))
		return
	}
	task.MetadataJSON = string(rawMeta)
	if _, err := r.deps.Cron.UpdateCronTask(ctx, task); err != nil {
		r.lg.Warn("update cron_task failed", loggateway.Str("task_id", task.ID), loggateway.Err(err))
	}
}

func (r *Runner) finishTaskRun(
	ctx context.Context,
	taskID, runID, started, trigger string,
	cfg cronTaskConfig,
	outcome runOutcome,
) {
	task, err := r.deps.Cron.GetCronTask(ctx, taskID)
	if err != nil {
		finished := time.Now().UTC().Format(time.RFC3339)
		if uerr := r.deps.Cron.UpdateCronTaskRun(ctx, runID, "failure", finished, "{}", err.Error()); uerr != nil {
			r.lg.Warn("update cron_task_run on reload failure failed", loggateway.Str("task_id", taskID), loggateway.Str("run_id", runID), loggateway.Err(uerr))
		}
		r.lg.Warn("reload task before finalize failed", loggateway.Str("task_id", taskID), loggateway.Str("run_id", runID), loggateway.Err(err))
		return
	}
	meta := parseCronTaskMetadata(task.MetadataJSON, r.lg)
	r.finalizeRun(ctx, runID, task, cfg, meta, started, trigger, outcome)
}

func (r *Runner) executeTask(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, _ cronTaskMetadata, now time.Time, trigger string) string {
	if trigger == "" {
		trigger = "schedule"
	}
	unlock := r.lockTask(task.ID)
	defer unlock()

	// C-26: cross-instance lease (Postgres advisory lock when DB is wired).
	releaseLease, ok := r.acquireTaskLease(ctx, task.ID)
	if !ok {
		r.lg.Info("cron task skipped: lease held by another instance",
			loggateway.StepID("cron.lease_skip"),
			loggateway.Str("task_id", task.ID),
			loggateway.Str("trigger", trigger))
		return ""
	}
	defer releaseLease()

	runID, started, ok := r.insertPendingRun(ctx, task.ID, trigger, now)
	if !ok {
		return ""
	}
	outcome := r.runDispatch(ctx, task, cfg)
	r.finishTaskRun(ctx, task.ID, runID, started, trigger, cfg, outcome)
	return runID
}

func (r *Runner) runManualTask(ctx context.Context, taskID, runID, started string, cfg cronTaskConfig) {
	unlock := r.lockTask(taskID)
	defer unlock()

	releaseLease, ok := r.acquireTaskLease(ctx, taskID)
	if !ok {
		finished := time.Now().UTC().Format(time.RFC3339)
		if uerr := r.deps.Cron.UpdateCronTaskRun(ctx, runID, "skipped", finished, "{}", "lease held by another instance"); uerr != nil {
			r.lg.Warn("update cron_task_run on lease skip failed", loggateway.Str("task_id", taskID), loggateway.Str("run_id", runID), loggateway.Err(uerr))
		}
		return
	}
	defer releaseLease()

	task, err := r.deps.Cron.GetCronTask(ctx, taskID)
	if err != nil {
		finished := time.Now().UTC().Format(time.RFC3339)
		if uerr := r.deps.Cron.UpdateCronTaskRun(ctx, runID, "failure", finished, "{}", err.Error()); uerr != nil {
			r.lg.Warn("update cron_task_run on manual reload failure failed", loggateway.Str("task_id", taskID), loggateway.Str("run_id", runID), loggateway.Err(uerr))
		}
		return
	}
	outcome := r.runDispatch(ctx, task, cfg)
	r.finishTaskRun(ctx, taskID, runID, started, "manual", cfg, outcome)
}

func (r *Runner) acquireTaskLease(ctx context.Context, taskID string) (release func(), ok bool) {
	if r == nil {
		return func() {}, true
	}
	lease := r.lease
	if lease == nil {
		lease = alwaysHeldLease{}
	}
	return lease.TryAcquire(ctx, taskID)
}

func isSessionBusyErr(err error) bool {
	return errors.Is(err, biz.ErrCronSessionBusy)
}

// recordPreExecuteOutcome writes a cron_task_run without dispatch, using the same finalize path as executeTask.
// status must be "failure" (execution-level failure, counts toward dead letter),
// "skipped" / "skipped_missed" (pre-dispatch skips, never count toward dead letter).
// 跳过一族优先折叠：若该任务最新一行执行记录与本行「状态+原因」逐字节一致（H11/orca
// skipped 折叠纪律），则原地累加 fold_count、不再新增行，避免每 tick 刷屏。
func (r *Runner) recordPreExecuteOutcome(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, now time.Time, status, message string) {
	switch status {
	case "failure", "skipped", cronRunStatusSkippedMissed:
	default:
		status = "failure"
	}
	unlock := r.lockTask(task.ID)
	defer unlock()

	runID, started, foldBase := "", now.UTC().Format(time.RFC3339), 0
	if status != "failure" {
		if prev, ok := r.latestFoldableRun(ctx, task.ID, status, message); ok {
			runID, started, foldBase = prev.ID, prev.StartedAt, prevFoldCount(prev)
		}
	}
	if runID == "" {
		var ok bool
		runID, started, ok = r.insertPendingRun(ctx, task.ID, "schedule", now)
		if !ok {
			r.lg.Warn("insert cron_task_run failed", loggateway.Str("task_id", task.ID), loggateway.Str("status", status), loggateway.Str("error", message))
			return
		}
	}
	r.finishTaskRun(ctx, task.ID, runID, started, "schedule", cfg, runOutcome{status: status, errMsg: message, foldBase: foldBase})
}

// latestFoldableRun 返回可折叠的最新执行行：状态与原因逐字节一致才折叠
// （orca skipped folding 的字节级同构纪律——原因串任何漂移都促生新行，保证语义可分辨）。
func (r *Runner) latestFoldableRun(ctx context.Context, taskID, status, message string) (biz.CronTaskRun, bool) {
	runs, _, err := r.deps.Cron.ListCronTaskRuns(ctx, biz.CronTaskRunQuery{TaskID: taskID, Limit: 1})
	if err != nil {
		r.lg.Warn("查询最近 cron_task_run 失败，跳过折叠", loggateway.Str("task_id", taskID), loggateway.Err(err))
		return biz.CronTaskRun{}, false
	}
	if len(runs) == 0 {
		return biz.CronTaskRun{}, false
	}
	latest := runs[0]
	if latest.Status == status && latest.ErrorMessage == message {
		return latest, true
	}
	return biz.CronTaskRun{}, false
}

// prevFoldCount 读取一行执行记录已代表的执行次数（无 fold_count 字段时视为 1）。
func prevFoldCount(run biz.CronTaskRun) int {
	if run.OutputJSON == "" {
		return 1
	}
	var out struct {
		FoldCount int `json:"fold_count"`
	}
	if err := json.Unmarshal([]byte(run.OutputJSON), &out); err == nil && out.FoldCount > 1 {
		return out.FoldCount
	}
	return 1
}

// recordUnevaluable mirrors orca's recordUnevaluableAutomation: 配置/调度不可求值
// （表达式无解、配置解析失败等静态问题）记 skipped 原地等修复——连续同字节原因折叠
// 进同一行，不推进死信计数。只有真实「执行失败」才计入 FailureCount 并可能致死。
func (r *Runner) recordUnevaluable(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, now time.Time, message string) {
	r.recordPreExecuteOutcome(ctx, task, cfg, now, "skipped", message)
}
