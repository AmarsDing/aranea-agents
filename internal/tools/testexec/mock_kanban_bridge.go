package testexec

import (
	"context"

	"aranea-agents/internal/tools/kanban"
)

// mockBridge is an in-memory mock that satisfies kanban.Bridge.
// It returns minimal stub responses for all operations.
type mockBridge struct{}

func (m *mockBridge) Show(ctx context.Context, taskID string) (map[string]any, error) {
	return map[string]any{"id": taskID, "status": "active"}, nil
}

func (m *mockBridge) List(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error) {
	return []map[string]any{}, nil
}

func (m *mockBridge) Complete(ctx context.Context, taskID, summary, output, metadata string) (map[string]any, error) {
	return map[string]any{"id": taskID, "status": "completed"}, nil
}

func (m *mockBridge) Block(ctx context.Context, taskID, reason, metadata string) (map[string]any, error) {
	return map[string]any{"id": taskID, "status": "blocked"}, nil
}

func (m *mockBridge) Unblock(ctx context.Context, taskID, comment string) (map[string]any, error) {
	return map[string]any{"id": taskID, "status": "active"}, nil
}

func (m *mockBridge) Heartbeat(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error) {
	return map[string]any{"id": taskID, "status": "active"}, nil
}

func (m *mockBridge) Comment(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error) {
	return map[string]any{"id": taskID, "comment_posted": true}, nil
}

func (m *mockBridge) Create(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
	return map[string]any{"id": "mock-task-1", "title": title, "status": "active"}, nil
}

func (m *mockBridge) Link(ctx context.Context, parentTaskID, childTaskID string) error {
	return nil
}

// Ensure mockBridge satisfies kanban.Bridge at compile time.
var _ kanban.Bridge = (*mockBridge)(nil)
