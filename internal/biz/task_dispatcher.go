package biz

import (
	"context"
	"time"

	"aranea-agents/internal/event"
)

const defaultDispatchInterval = 30 * time.Second

// TaskDispatchAgentRunner claims pending tasks and notifies assignees (Hermes dispatcher subset).
type TaskDispatchAgentRunner interface {
	DispatchTask(ctx context.Context, task *GraphTask, agentKey string) error
}

// TaskDispatcher periodically scans pending tasks and auto-claims static assignments.
type TaskDispatcher struct {
	tasks    *TaskUsecase
	runner   TaskDispatchAgentRunner
	interval time.Duration
	stop     chan struct{}
}

func NewTaskDispatcher(tasks *TaskUsecase, runner TaskDispatchAgentRunner) *TaskDispatcher {
	return &TaskDispatcher{
		tasks:    tasks,
		runner:   runner,
		interval: defaultDispatchInterval,
		stop:     make(chan struct{}),
	}
}

func (d *TaskDispatcher) Start() {
	if d == nil || d.tasks == nil {
		return
	}
	go d.loop()
}

func (d *TaskDispatcher) Stop() {
	if d == nil {
		return
	}
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
}

func (d *TaskDispatcher) loop() {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			if err := d.tick(ctx); err != nil {
				event.SysLogWarn("system.task.dispatcher_tick_fail", "task dispatcher tick failed",
					event.P("error", err.Error()))
			}
			cancel()
		}
	}
}

func (d *TaskDispatcher) tick(ctx context.Context) error {
	if d.tasks == nil {
		return nil
	}
	if err := d.tasks.CheckTimeouts(ctx); err != nil {
		event.SysLogWarn("system.task.check_timeout_fail", "task check timeouts failed",
			event.P("error", err.Error()))
	}
	items, err := d.tasks.ListPendingTasks(ctx, 50)
	if err != nil {
		return err
	}
	for _, task := range items {
		if task == nil {
			continue
		}
		if !d.tasks.isTaskReadyForDispatch(ctx, task) {
			continue
		}
		agentKey := d.tasks.resolveDispatchAssignee(ctx, task)
		if agentKey == "" {
			continue
		}
		claimed, err := d.tasks.ClaimTask(ctx, task.TaskID, agentKey)
		if err != nil {
			event.SysLogWarn("system.task.claim_fail", "task dispatch claim failed",
				event.P("task_id", task.TaskID), event.P("agent", agentKey), event.P("error", err.Error()))
			continue
		}
		if claimed == nil {
			continue
		}
		if d.runner != nil {
			if err := d.runner.DispatchTask(ctx, claimed, agentKey); err != nil {
				event.SysLogWarn("system.task.dispatch_run_fail", "task dispatch run failed",
					event.P("task_id", claimed.TaskID), event.P("agent", agentKey), event.P("error", err.Error()))
				d.tasks.ReleaseClaim(ctx, claimed.TaskID)
			}
		}
	}
	return nil
}
