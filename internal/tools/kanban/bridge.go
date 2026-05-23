package kanban

import "context"

// Bridge is the task-board API surface for kanban_* agent tools (M54).
type Bridge interface {
	Show(ctx context.Context, taskID string) (map[string]any, error)
	List(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error)
	Complete(ctx context.Context, taskID, summary, output, metadata string) (map[string]any, error)
	Block(ctx context.Context, taskID, reason, metadata string) (map[string]any, error)
	Unblock(ctx context.Context, taskID, comment string) (map[string]any, error)
	Heartbeat(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error)
	Comment(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error)
	Create(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error)
	Link(ctx context.Context, parentTaskID, childTaskID string) error
}

type bridgeKey struct{}

func WithBridge(ctx context.Context, b Bridge) context.Context {
	return context.WithValue(ctx, bridgeKey{}, b)
}

func BridgeFromContext(ctx context.Context) Bridge {
	if b, ok := ctx.Value(bridgeKey{}).(Bridge); ok {
		return b
	}
	return globalBridge
}

var globalBridge Bridge

func SetGlobalBridge(b Bridge) {
	globalBridge = b
}

func TaskIDFromEnv() string {
	return envOrEmpty("ARANEA_TASK_ID")
}

func ExecutionIDFromEnv() string {
	return envOrEmpty("ARANEA_EXECUTION_ID")
}

func envOrEmpty(key string) string {
	// resolved in tools.go via os.Getenv to keep bridge testable
	return lookupEnv(key)
}
