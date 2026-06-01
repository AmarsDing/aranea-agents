package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultDispatchInterval = 30 * time.Second

type TaskDispatchAgentRunner interface {
	DispatchTask(ctx context.Context, task *GraphTask, agentKey string) error
}

type TaskDispatchReader interface {
	CheckTimeouts(ctx context.Context) error
	ListPendingTasks(ctx context.Context, limit int) ([]*GraphTask, error)
	BatchResolveReadiness(ctx context.Context, tasks []*GraphTask) map[string]bool
	ResolveDispatchAssignee(ctx context.Context, task *GraphTask) string
	ClaimTask(ctx context.Context, taskID string, agentKey string) (*GraphTask, error)
	ReleaseClaim(ctx context.Context, taskID string)
}

type TaskDispatcher struct {
	tasks    TaskDispatchReader
	runner   TaskDispatchAgentRunner
	interval time.Duration
	cancel   context.CancelFunc
	lg       loggateway.Logger
}

func NewTaskDispatcher(tasks TaskDispatchReader, runner TaskDispatchAgentRunner, lg loggateway.Logger) *TaskDispatcher {
	return &TaskDispatcher{
		tasks:    tasks,
		runner:   runner,
		interval: defaultDispatchInterval,
		lg:       lg,
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
				d.lg.Warn("task dispatcher tick failed",
					loggateway.StepID("task.dispatcher_tick_fail"), loggateway.Err(err))
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
		d.lg.Warn("task check timeouts failed",
			loggateway.StepID("task.check_timeout_fail"), loggateway.Err(err))
	}
	items, err := d.tasks.ListPendingTasks(ctx, 50)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	readyMap := d.tasks.BatchResolveReadiness(ctx, items)
	dispatched := 0
	for _, task := range items {
		if task == nil {
			continue
		}
		if !readyMap[task.TaskID] {
			continue
		}
		agentKey := d.tasks.ResolveDispatchAssignee(ctx, task)
		if agentKey == "" {
			continue
		}
		claimed, err := d.tasks.ClaimTask(ctx, task.TaskID, agentKey)
		if err != nil {
			d.lg.Warn("task dispatch claim failed",
				loggateway.StepID("task.claim_fail"), loggateway.Str("task_id", task.TaskID), loggateway.Str("agent", agentKey), loggateway.Err(err))
			continue
		}
		if claimed == nil {
			continue
		}
		if d.runner != nil {
			if err := d.runner.DispatchTask(ctx, claimed, agentKey); err != nil {
				d.lg.Warn("task dispatch run failed",
					loggateway.StepID("task.dispatch_run_fail"), loggateway.Str("task_id", claimed.TaskID), loggateway.Str("agent", agentKey), loggateway.Err(err))
				d.tasks.ReleaseClaim(ctx, claimed.TaskID)
				continue
			}
		}
		dispatched++
	}
	readyCount := 0
	for _, ready := range readyMap {
		if ready {
			readyCount++
		}
	}
	d.lg.Info("task dispatcher tick completed",
		loggateway.StepID("task.dispatcher_tick"),
		loggateway.Int("pending_count", len(items)),
		loggateway.Int("ready_count", readyCount),
		loggateway.Int("dispatched_count", dispatched))
	return nil
}
