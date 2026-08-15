package kanban

import "context"

type BridgeReader interface {
	Show(ctx context.Context, taskID string) (map[string]any, error)
	List(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error)
}

type BridgeWriter interface {
	// Complete 提交任务结果；agentKey 为提交者（空串表示无提交者上下文，
	// biz 层将跳过 assignee 守卫），非空时强制校验其为任务认领者。
	Complete(ctx context.Context, taskID, agentKey, summary, output, metadata string) (map[string]any, error)
	Block(ctx context.Context, taskID, reason, metadata string) (map[string]any, error)
	Unblock(ctx context.Context, taskID, comment string) (map[string]any, error)
	Heartbeat(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error)
}

type BridgeLifecycle interface {
	Comment(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error)
	Create(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error)
	Link(ctx context.Context, parentTaskID, childTaskID string) error
}

type Bridge interface {
	BridgeReader
	BridgeWriter
	BridgeLifecycle
}

func TaskIDFromEnv() string {
	return envOrEmpty("ARANEA_TASK_ID")
}

func ExecutionIDFromEnv() string {
	return envOrEmpty("ARANEA_EXECUTION_ID")
}

func envOrEmpty(key string) string {
	return lookupEnv(key)
}
