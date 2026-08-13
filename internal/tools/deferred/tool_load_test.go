package deferred

import (
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func newWebFetchTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "fetched", nil },
		trpcfunction.WithName("web_fetch"),
		trpcfunction.WithDescription("Fetch web content"),
	)
}

func TestToolLoadTool_Declaration(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
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
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()
	mgr.RegisterTool("web_fetch", newWebFetchTool())

	ctx := withTestInvocation(context.Background())

	// 初始状态：未激活
	if mgr.IsActivated(ctx, "web_fetch") {
		t.Fatal("expected web_fetch not activated initially")
	}

	// 加载工具
	input := toolLoadInput{ToolName: "web_fetch"}
	args, _ := json.Marshal(input)
	result, err := tool.Call(ctx, args)
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
	// 返回完整声明（含 schema）
	if output.Schema == nil {
		t.Error("expected full schema in response")
	}

	// 激活后：IsActivated 返回 true
	if !mgr.IsActivated(ctx, "web_fetch") {
		t.Error("expected web_fetch activated after load")
	}
}

func TestToolLoadTool_LoadNonExistentTool(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)

	ctx := withTestInvocation(context.Background())
	input := toolLoadInput{ToolName: "nonexistent_tool"}
	args, _ := json.Marshal(input)
	result, err := tool.Call(ctx, args)
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
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()
	mgr.RegisterTool("web_fetch", newWebFetchTool())

	ctx := withTestInvocation(context.Background())

	// 第一次加载
	input := toolLoadInput{ToolName: "web_fetch"}
	args, _ := json.Marshal(input)
	_, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("first Call failed: %v", err)
	}

	// 第二次加载（幂等）
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("second Call failed: %v", err)
	}
	output := result.(toolLoadOutput)
	if !output.Success {
		t.Errorf("expected success for already-activated tool, got error: %s", output.Error)
	}
}

func TestToolLoadTool_LoadMultipleTools(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()
	mgr.RegisterTool("web_fetch", newWebFetchTool())
	mgr.RegisterTool("shell_exec", &mockTool{name: "shell_exec"})

	ctx := withTestInvocation(context.Background())

	// 加载第一个
	input1 := toolLoadInput{ToolName: "web_fetch"}
	args1, _ := json.Marshal(input1)
	if _, err := tool.Call(ctx, args1); err != nil {
		t.Fatalf("Call web_fetch failed: %v", err)
	}

	// 加载第二个
	input2 := toolLoadInput{ToolName: "shell_exec"}
	args2, _ := json.Marshal(input2)
	if _, err := tool.Call(ctx, args2); err != nil {
		t.Fatalf("Call shell_exec failed: %v", err)
	}

	// 两个都已激活
	if !mgr.IsActivated(ctx, "web_fetch") {
		t.Error("expected web_fetch activated")
	}
	if !mgr.IsActivated(ctx, "shell_exec") {
		t.Error("expected shell_exec activated")
	}
}

func TestToolLoadTool_RuntimeNameOverridesSchemaName(t *testing.T) {
	// ToolSet 前缀工具：catalog 用运行时名（file_read_file），
	// 激活后返回的 schema Name 必须是运行时名（模型按此名调用）。
	catalog := []DeferredToolEntry{
		{Name: "file_read_file", BaseName: "read_file", Description: "Read a file", Category: "filesystem"},
	}
	tool := NewToolLoadTool(catalog)
	mgr := tool.Manager()
	mgr.RegisterTool("file_read_file", &mockTool{name: "read_file"})

	ctx := withTestInvocation(context.Background())
	input := toolLoadInput{ToolName: "file_read_file"}
	args, _ := json.Marshal(input)
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	output := result.(toolLoadOutput)
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Schema == nil || output.Schema.Name != "file_read_file" {
		t.Fatalf("expected schema name file_read_file, got %+v", output.Schema)
	}
	// 基础名同步激活（DeferredCallableTool 门禁按基础名检查）
	if !mgr.IsActivated(ctx, "read_file") {
		t.Error("expected base name read_file also activated")
	}
}

func TestToolLoadTool_EmptyToolName(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	tool := NewToolLoadTool(catalog)

	ctx := withTestInvocation(context.Background())
	input := toolLoadInput{ToolName: ""}
	args, _ := json.Marshal(input)
	result, err := tool.Call(ctx, args)
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
