package biz

import "context"

// GraphTaskStatusPublisher emits task lifecycle updates to realtime observers (WS / orchestration).
type GraphTaskStatusPublisher interface {
	PublishGraphTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any)
}

// GraphTaskCoordinator hooks graph runtime events to the task board (M54).
type GraphTaskCoordinator interface {
	GraphTaskStatusPublisher
	OnGraphNodeStart(ctx context.Context, exec *GraphExecution, node *NodeDef, meta NodeTaskMeta, inputPreview string) error
	OnTaskCompleted(ctx context.Context, task *GraphTask) error
}

// TaskCompletionHandler is invoked when a task reaches a terminal success state.
type TaskCompletionHandler interface {
	OnTaskCompleted(ctx context.Context, task *GraphTask) error
}
