package cronrunner

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type runOutcome struct {
	result cronDispatchResult
	status string
	errMsg string
}

func (r *Runner) lockTask(taskID string) func() {
	v, _ := r.taskMu.LoadOrStore(taskID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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
	result, execErr := r.dispatchWithRetry(ctx, task, cfg)
	elapsed := time.Since(startedTime)
	cronJobDuration.WithLabelValues(task.ID).Observe(elapsed.Seconds())

	if execErr == nil {
		return runOutcome{result: result, status: "success"}
	}
	if isSessionBusyErr(execErr) {
		return runOutcome{result: result, status: "skipped", errMsg: execErr.Error()}
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
		"run_id":           outcome.result.AgentMessageID,
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
		if !manual {
			meta.FailureCount = 0
		}
	case "skipped":
		meta.LastError = outcome.errMsg
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
				r.lg.Warn("定时任务进入死信", loggateway.Str("job_id", task.ID), loggateway.Str("task_key", task.TaskKey), loggateway.Int("failure_count", meta.FailureCount))
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
		_ = r.deps.Cron.UpdateCronTaskRun(ctx, runID, "failure", finished, "{}", err.Error())
		r.lg.Warn("reload task before finalize failed", loggateway.Str("task_id", taskID), loggateway.Str("run_id", runID), loggateway.Err(err))
		return
	}
	meta := parseCronTaskMetadata(task.MetadataJSON)
	r.finalizeRun(ctx, runID, task, cfg, meta, started, trigger, outcome)
}

func (r *Runner) executeTask(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, _ cronTaskMetadata, now time.Time, trigger string) string {
	if trigger == "" {
		trigger = "schedule"
	}
	unlock := r.lockTask(task.ID)
	defer unlock()

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

	task, err := r.deps.Cron.GetCronTask(ctx, taskID)
	if err != nil {
		finished := time.Now().UTC().Format(time.RFC3339)
		_ = r.deps.Cron.UpdateCronTaskRun(ctx, runID, "failure", finished, "{}", err.Error())
		return
	}
	outcome := r.runDispatch(ctx, task, cfg)
	r.finishTaskRun(ctx, taskID, runID, started, "manual", cfg, outcome)
}

func isSessionBusyErr(err error) bool {
	return errors.Is(err, biz.ErrCronSessionBusy)
}

// recordPreExecuteOutcome writes a cron_task_run without dispatch, using the same finalize path as executeTask.
// status must be "failure" (config/schedule errors, counts toward dead letter) or "skipped" (transient pre-dispatch abort).
func (r *Runner) recordPreExecuteOutcome(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, now time.Time, status, message string) {
	if status != "failure" && status != "skipped" {
		status = "failure"
	}
	unlock := r.lockTask(task.ID)
	defer unlock()

	runID, started, ok := r.insertPendingRun(ctx, task.ID, "schedule", now)
	if !ok {
		r.lg.Warn("insert cron_task_run failed", loggateway.Str("task_id", task.ID), loggateway.Str("status", status), loggateway.Str("error", message))
		return
	}
	r.finishTaskRun(ctx, task.ID, runID, started, "schedule", cfg, runOutcome{status: status, errMsg: message})
}

// recordScheduleFailure records invalid config/schedule as a failed run (not skipped).
func (r *Runner) recordScheduleFailure(ctx context.Context, task biz.CronTask, cfg cronTaskConfig, now time.Time, message string) {
	r.recordPreExecuteOutcome(ctx, task, cfg, now, "failure", message)
}
