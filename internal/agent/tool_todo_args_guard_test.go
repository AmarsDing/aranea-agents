package agent

import (
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestTodoArgsGuard_stripsTodosFromNonTodoWrite(t *testing.T) {
	hook := newTodoArgsGuardBeforeHook(nil)
	args, _ := json.Marshal(map[string]any{
		"file_path": "/tmp/test.txt",
		"todos":     []map[string]string{{"content": "task1", "status": "pending"}},
	})
	btArgs := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: args}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	// P1-3: 剥离结果经 ModifiedArguments 返回（框架唯一写回通道）。
	if res.ModifiedArguments == nil {
		t.Fatal("stripped args must be returned via ModifiedArguments")
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(res.ModifiedArguments, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["todos"]; ok {
		t.Fatalf("todos not stripped from non-todo_write tool: %v", string(out["todos"]))
	}
	if string(out["file_path"]) != `"/tmp/test.txt"` {
		t.Fatalf("file_path mutated: %v", string(out["file_path"]))
	}
}

func TestTodoArgsGuard_preservesTodosForTodoWrite(t *testing.T) {
	hook := newTodoArgsGuardBeforeHook(nil)
	args, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{{"content": "task1", "status": "pending"}},
	})
	btArgs := &trpctool.BeforeToolArgs{ToolName: "todo_write", Arguments: args}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(btArgs.Arguments, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["todos"]; !ok {
		t.Fatalf("todos should NOT be stripped for todo_write tool")
	}
}

func TestTodoArgsGuard_invalidJSON(t *testing.T) {
	hook := newTodoArgsGuardBeforeHook(nil)
	btArgs := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: []byte("not-json")}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	if string(btArgs.Arguments) != "not-json" {
		t.Fatalf("invalid JSON should be passed through unchanged")
	}
}

func TestTodoArgsGuard_emptyArgs(t *testing.T) {
	hook := newTodoArgsGuardBeforeHook(nil)
	btArgs := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: nil}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	if btArgs.Arguments != nil {
		t.Fatalf("nil args should remain nil")
	}
}

func TestTodoArgsGuard_noTodosField(t *testing.T) {
	hook := newTodoArgsGuardBeforeHook(nil)
	args, _ := json.Marshal(map[string]any{
		"file_path": "/tmp/test.txt",
	})
	btArgs := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: args}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	if string(btArgs.Arguments) != string(args) {
		t.Fatalf("args without todos should be unchanged")
	}
}
