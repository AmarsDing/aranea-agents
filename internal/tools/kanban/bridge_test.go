package kanban

import (
	"context"
	"testing"
)

type stubBridge struct {
	showFn       func(ctx context.Context, taskID string) (map[string]any, error)
	listFn       func(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error)
	completeFn   func(ctx context.Context, taskID, summary, output, metadata string) (map[string]any, error)
	blockFn      func(ctx context.Context, taskID, reason, metadata string) (map[string]any, error)
	unblockFn    func(ctx context.Context, taskID, comment string) (map[string]any, error)
	heartbeatFn  func(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error)
	commentFn    func(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error)
	createFn     func(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error)
	linkFn       func(ctx context.Context, parentTaskID, childTaskID string) error
}

func (s *stubBridge) Show(ctx context.Context, taskID string) (map[string]any, error) {
	return s.showFn(ctx, taskID)
}

func (s *stubBridge) List(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error) {
	return s.listFn(ctx, executionID, status, limit)
}

func (s *stubBridge) Complete(ctx context.Context, taskID, summary, output, metadata string) (map[string]any, error) {
	return s.completeFn(ctx, taskID, summary, output, metadata)
}

func (s *stubBridge) Block(ctx context.Context, taskID, reason, metadata string) (map[string]any, error) {
	return s.blockFn(ctx, taskID, reason, metadata)
}

func (s *stubBridge) Unblock(ctx context.Context, taskID, comment string) (map[string]any, error) {
	return s.unblockFn(ctx, taskID, comment)
}

func (s *stubBridge) Heartbeat(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error) {
	return s.heartbeatFn(ctx, taskID, agentKey, metadata)
}

func (s *stubBridge) Comment(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error) {
	return s.commentFn(ctx, taskID, author, body, commentType)
}

func (s *stubBridge) Create(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
	return s.createFn(ctx, executionID, nodeID, title, assignee, input, parentIDs)
}

func (s *stubBridge) Link(ctx context.Context, parentTaskID, childTaskID string) error {
	return s.linkFn(ctx, parentTaskID, childTaskID)
}

var _ Bridge = (*stubBridge)(nil)

func TestBridge_subInterfaceAssignment(t *testing.T) {
	t.Parallel()

	var b Bridge = &stubBridge{}

	var _ BridgeReader = b
	var _ BridgeWriter = b
	var _ BridgeLifecycle = b

	reader := BridgeReader(b)
	if reader == nil {
		t.Fatal("Bridge should be assignable to BridgeReader")
	}

	writer := BridgeWriter(b)
	if writer == nil {
		t.Fatal("Bridge should be assignable to BridgeWriter")
	}

	lifecycle := BridgeLifecycle(b)
	if lifecycle == nil {
		t.Fatal("Bridge should be assignable to BridgeLifecycle")
	}
}

func TestBridgeReader_interfaceMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	called := false
	b := &stubBridge{
		showFn: func(_ context.Context, taskID string) (map[string]any, error) {
			called = true
			if taskID != "t1" {
				t.Fatalf("expected taskID t1, got %s", taskID)
			}
			return map[string]any{"id": "t1"}, nil
		},
		listFn: func(_ context.Context, executionID, status string, limit int) ([]map[string]any, error) {
			if executionID != "e1" {
				t.Fatalf("expected executionID e1, got %s", executionID)
			}
			return []map[string]any{{"id": "t1"}}, nil
		},
	}

	var reader BridgeReader = b
	result, err := reader.Show(ctx, "t1")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !called {
		t.Fatal("Show was not called")
	}
	if result["id"] != "t1" {
		t.Fatalf("expected id t1, got %v", result["id"])
	}

	items, err := reader.List(ctx, "e1", "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestBridgeWriter_interfaceMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := &stubBridge{
		completeFn: func(_ context.Context, taskID, summary, output, metadata string) (map[string]any, error) {
			return map[string]any{"task_id": taskID, "status": "completed"}, nil
		},
		blockFn: func(_ context.Context, taskID, reason, metadata string) (map[string]any, error) {
			return map[string]any{"task_id": taskID, "status": "blocked"}, nil
		},
		unblockFn: func(_ context.Context, taskID, comment string) (map[string]any, error) {
			return map[string]any{"task_id": taskID, "status": "pending"}, nil
		},
		heartbeatFn: func(_ context.Context, taskID, agentKey, metadata string) (map[string]any, error) {
			return map[string]any{"task_id": taskID, "alive": true}, nil
		},
	}

	var writer BridgeWriter = b

	result, err := writer.Complete(ctx, "t1", "done", "", "")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result["status"] != "completed" {
		t.Fatalf("expected completed, got %v", result["status"])
	}

	result, err = writer.Block(ctx, "t1", "need input", "")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if result["status"] != "blocked" {
		t.Fatalf("expected blocked, got %v", result["status"])
	}

	result, err = writer.Unblock(ctx, "t1", "resolved")
	if err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if result["status"] != "pending" {
		t.Fatalf("expected pending, got %v", result["status"])
	}

	result, err = writer.Heartbeat(ctx, "t1", "agent-1", "")
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if result["alive"] != true {
		t.Fatalf("expected alive true, got %v", result["alive"])
	}
}

func TestBridgeLifecycle_interfaceMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := &stubBridge{
		commentFn: func(_ context.Context, taskID, author, body, commentType string) (map[string]any, error) {
			return map[string]any{"task_id": taskID, "author": author}, nil
		},
		createFn: func(_ context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
			return map[string]any{"title": title, "execution_id": executionID}, nil
		},
		linkFn: func(_ context.Context, parentTaskID, childTaskID string) error {
			return nil
		},
	}

	var lc BridgeLifecycle = b

	result, err := lc.Comment(ctx, "t1", "agent", "hello", "info")
	if err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if result["author"] != "agent" {
		t.Fatalf("expected author agent, got %v", result["author"])
	}

	result, err = lc.Create(ctx, "e1", "n1", "title", "assignee", "input", []string{"p1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result["title"] != "title" {
		t.Fatalf("expected title, got %v", result["title"])
	}

	if err := lc.Link(ctx, "p1", "c1"); err != nil {
		t.Fatalf("Link: %v", err)
	}
}

func TestTaskIDFromEnv_set(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "task-42")
	got := TaskIDFromEnv()
	if got != "task-42" {
		t.Fatalf("expected task-42, got %q", got)
	}
}

func TestTaskIDFromEnv_empty(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	got := TaskIDFromEnv()
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExecutionIDFromEnv_set(t *testing.T) {
	t.Setenv("ARANEA_EXECUTION_ID", "exec-99")
	got := ExecutionIDFromEnv()
	if got != "exec-99" {
		t.Fatalf("expected exec-99, got %q", got)
	}
}

func TestExecutionIDFromEnv_empty(t *testing.T) {
	t.Setenv("ARANEA_EXECUTION_ID", "")
	got := ExecutionIDFromEnv()
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
