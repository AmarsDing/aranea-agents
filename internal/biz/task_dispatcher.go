package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
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
	log      *log.Helper
}

func NewTaskDispatcher(tasks *TaskUsecase, runner TaskDispatchAgentRunner) *TaskDispatcher {
	return &TaskDispatcher{
		tasks:    tasks,
		runner:   runner,
		interval: defaultDispatchInterval,
		stop:     make(chan struct{}),
		log:      log.NewHelper(log.DefaultLogger),
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
				d.log.Warnf("task dispatcher tick: %v", err)
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
		d.log.Warnf("task check timeouts: %v", err)
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
			d.log.Warnf("task dispatch claim: task_id=%s agent=%s: %v", task.TaskID, agentKey, err)
			continue
		}
		if claimed == nil {
			continue
		}
		if d.runner != nil {
			if err := d.runner.DispatchTask(ctx, claimed, agentKey); err != nil {
				d.log.Warnf("task dispatch run: task_id=%s agent=%s: %v", claimed.TaskID, agentKey, err)
			}
		}
	}
	return nil
}
