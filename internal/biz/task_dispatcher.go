package biz

import (
	"context"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const defaultDispatchInterval = 30 * time.Second

// TaskDispatchAgentRunner claims pending tasks and notifies assignees (Hermes dispatcher subset).
type TaskDispatchAgentRunner interface {
	DispatchTask(ctx context.Context, task *GraphTask, agentKey string) error
}

type TaskDispatchReader interface {
	CheckTimeouts(ctx context.Context) error
	ListPendingTasks(ctx context.Context, limit int) ([]*GraphTask, error)
	IsTaskReadyForDispatch(ctx context.Context, task *GraphTask) bool
	ResolveDispatchAssignee(ctx context.Context, task *GraphTask) string
	ClaimTask(ctx context.Context, taskID string, agentKey string) (*GraphTask, error)
	ReleaseClaim(ctx context.Context, taskID string)
}

type TaskDispatcher struct {
	tasks    TaskDispatchReader
	runner   TaskDispatchAgentRunner
	interval time.Duration
	cancel   context.CancelFunc
}

func NewTaskDispatcher(tasks TaskDispatchReader, runner TaskDispatchAgentRunner) *TaskDispatcher {
	return &TaskDispatcher{
		tasks:    tasks,
		runner:   runner,
		interval: defaultDispatchInterval,
	}
}

func (d *TaskDispatcher) Start() {
	if d == nil || d.tasks == nil {
		return
	}
	if d.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	safego.Go(context.Background(), "task_dispatcher.loop", func() { d.loop(ctx) })
}

func (d *TaskDispatcher) Stop() {
	if d == nil {
		return
	}
	if d.cancel != nil {
		d.cancel()
	}
}

func (d *TaskDispatcher) loop(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx, tickCancel := context.WithTimeout(ctx, 25*time.Second)
			if err := d.tick(tickCtx); err != nil {
				event.SysLogWarn("system.task.dispatcher_tick_fail", "task dispatcher tick failed",
					event.P("error", err.Error()))
			}
			tickCancel()
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
		if !d.tasks.IsTaskReadyForDispatch(ctx, task) {
			continue
		}
		agentKey := d.tasks.ResolveDispatchAssignee(ctx, task)
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
