package deferred

import (
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestToolLoadTool_Declaration(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", Description: "Fetch web content", Category: "web"},
		{Name: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	tool := NewToolLoadTool(catalog)
	decl := tool.Declaration()
	if decl == nil {
		t.Fatal("expected non-nil declaration")
	}
	if decl.Name != "tool_load" {
		t.Errorf("expected name=tool_load, got %s", decl.Name)
	}
	if decl.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestToolLoadTool_LoadExistingTool(t *testing.T) {
	catalog := []DeferredToolEntry{
		{
			Name:        "web_fetch",
			Description: "Fetch web content",
			Category:    "web",
			Factory: func(_ context.Context) (trpctool.Tool, error) {
				return &mockTool{name: "web_fetch"}, nil
			},
		},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()

	// 初始状态：未激活
	if mgr.IsActivated("web_fetch") {
		t.Fatal("expected web_fetch not activated initially")
	}

	// 加载工具
	input := toolLoadInput{ToolName: "web_fetch"}
	args, _ := json.Marshal(input)
	result, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	output, ok := result.(toolLoadOutput)
	if !ok {
		t.Fatalf("expected toolLoadOutput, got %T", result)
	}
	if !output.Success {
		t.Errorf("expected success, got error: %s", output.Error)
	}
	if output.ToolName != "web_fetch" {
		t.Errorf("expected tool_name=web_fetch, got %s", output.ToolName)
	}

	// 激活后：IsActivated 返回 true
	if !mgr.IsActivated("web_fetch") {
		t.Error("expected web_fetch activated after load")
	}
}

func TestToolLoadTool_LoadNonExistentTool(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)

	input := toolLoadInput{ToolName: "nonexistent_tool"}
	args, _ := json.Marshal(input)
	result, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	output, ok := result.(toolLoadOutput)
	if !ok {
		t.Fatalf("expected toolLoadOutput, got %T", result)
	}
	if output.Success {
		t.Error("expected failure for non-existent tool")
	}
	if output.Error == "" {
		t.Error("expected error message for non-existent tool")
	}
}

func TestToolLoadTool_LoadAlreadyActivatedTool(t *testing.T) {
	catalog := []DeferredToolEntry{
		{
			Name:        "web_fetch",
			Description: "Fetch web content",
			Category:    "web",
			Factory: func(_ context.Context) (trpctool.Tool, error) {
				return &mockTool{name: "web_fetch"}, nil
			},
		},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()

	// 第一次加载
	input := toolLoadInput{ToolName: "web_fetch"}
	args, _ := json.Marshal(input)
	_, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("first Call failed: %v", err)
	}

	// 第二次加载（幂等）
	result, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("second Call failed: %v", err)
	}
	output := result.(toolLoadOutput)
	if !output.Success {
		t.Errorf("expected success for already-activated tool, got error: %s", output.Error)
	}

	// Activate 计数只增一次（第二次直接返回缓存）
	if count := mgr.ActivateStats()["web_fetch"]; count != 1 {
		t.Errorf("expected activate count=1, got %d", count)
	}
}

func TestToolLoadTool_LoadMultipleTools(t *testing.T) {
	catalog := []DeferredToolEntry{
		{
			Name:        "web_fetch",
			Description: "Fetch web content",
			Category:    "web",
			Factory: func(_ context.Context) (trpctool.Tool, error) {
				return &mockTool{name: "web_fetch"}, nil
			},
		},
		{
			Name:        "shell_exec",
			Description: "Execute shell commands",
			Category:    "runtime",
			Factory: func(_ context.Context) (trpctool.Tool, error) {
				return &mockTool{name: "shell_exec"}, nil
			},
		},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()

	// 加载第一个
	input1 := toolLoadInput{ToolName: "web_fetch"}
	args1, _ := json.Marshal(input1)
	_, err := tool.Call(context.Background(), args1)
	if err != nil {
		t.Fatalf("Call web_fetch failed: %v", err)
	}

	// 加载第二个
	input2 := toolLoadInput{ToolName: "shell_exec"}
	args2, _ := json.Marshal(input2)
	_, err = tool.Call(context.Background(), args2)
	if err != nil {
		t.Fatalf("Call shell_exec failed: %v", err)
	}

	// 两个都已激活
	if !mgr.IsActivated("web_fetch") {
		t.Error("expected web_fetch activated")
	}
	if !mgr.IsActivated("shell_exec") {
		t.Error("expected shell_exec activated")
	}

	// ActivatedTools 返回 2 个
	activated := mgr.ActivatedTools()
	if len(activated) != 2 {
		t.Errorf("expected 2 activated tools, got %d", len(activated))
	}
}

func TestToolLoadTool_EmptyToolName(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)

	input := toolLoadInput{ToolName: ""}
	args, _ := json.Marshal(input)
	result, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	output := result.(toolLoadOutput)
	if output.Success {
		t.Error("expected failure for empty tool name")
	}
}

// mockTool implements trpctool.Tool for testing.
type mockTool struct {
	name string
}

func (m *mockTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: m.name, Description: "mock " + m.name}
}
