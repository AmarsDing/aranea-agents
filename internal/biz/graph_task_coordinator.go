package biz

import "context"

// TaskStatusPublisher emits task lifecycle updates to realtime observers (WS / orchestration).
type TaskStatusPublisher interface {
	PublishTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any)
}

// GraphTaskCoordinator hooks graph runtime events to the task board (M54).
type GraphTaskCoordinator interface {
	TaskStatusPublisher
	OnGraphNodeStart(ctx context.Context, exec *GraphExecution, node *NodeDef, inputPreview string) error
	OnTaskCompleted(ctx context.Context, task *GraphTask) error
}

// TaskCompletionHandler is invoked when a task reaches a terminal success state.
type TaskCompletionHandler interface {
	OnTaskCompleted(ctx context.Context, task *GraphTask) error
}
