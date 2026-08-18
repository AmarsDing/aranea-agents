package deferred

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type weatherInput struct {
	City string `json:"city" jsonschema:"description=City name,required"`
}

type weatherOutput struct {
	Temperature string `json:"temperature"`
	Condition   string `json:"condition"`
}

func createWeatherTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(_ context.Context, in weatherInput) (weatherOutput, error) {
			return weatherOutput{Temperature: "22°C", Condition: "sunny"}, nil
		},
		trpcfunction.WithName("weather_lookup"),
		trpcfunction.WithDescription("Look up weather for a city"),
	)
}

// buildTestCatalog 构建包含独立工具 + ToolSet 前缀工具的测试 catalog。
func buildTestCatalog() []DeferredToolEntry {
	return []DeferredToolEntry{
		{Name: "weather_lookup", BaseName: "weather_lookup", Description: "Look up weather for a city", Category: "weather"},
		{Name: "translate_text", BaseName: "translate_text", Description: "Translate text between languages", Category: "language"},
		{Name: "file_read_file", BaseName: "read_file", Description: "Read a file from disk", Category: "filesystem"},
	}
}

// newTestManager 创建注册了工具引用的 manager（模拟 assembleDeferredTools 行为）。
func newTestManager(catalog []DeferredToolEntry) *DeferredToolManager {
	manager := NewDeferredToolManager(catalog)
	manager.RegisterTool("weather_lookup", createWeatherTool())
	return manager
}

func TestToolSearch_ReturnsMatches(t *testing.T) {
	catalog := buildTestCatalog()
	searchTool := NewToolSearchTool(catalog)

	result, err := searchTool.Call(context.Background(), []byte(`{"query": "weather"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Tools))
	}
	if output.Tools[0].Name != "weather_lookup" {
		t.Fatalf("expected weather_lookup, got %s", output.Tools[0].Name)
	}
}

func TestToolSearch_NoAutoActivation(t *testing.T) {
	catalog := buildTestCatalog()
	searchTool := NewToolSearchTool(catalog)
	manager := searchTool.Manager()

	ctx := withTestInvocation(context.Background())
	if manager.IsActivated(ctx, "weather_lookup") {
		t.Fatal("weather_lookup should not be activated before search")
	}

	_, err := searchTool.Call(ctx, []byte(`{"query": "weather"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// WP-4：搜索不再自动激活，必须显式 tool_load
	if manager.IsActivated(ctx, "weather_lookup") {
		t.Fatal("weather_lookup must NOT be activated by search alone")
	}
}

func TestToolSearch_NoMatch(t *testing.T) {
	catalog := buildTestCatalog()
	searchTool := NewToolSearchTool(catalog)

	result, err := searchTool.Call(context.Background(), []byte(`{"query": "nonexistent_tool_xyz"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 0 {
		t.Fatalf("expected 0 results, got %d", len(output.Tools))
	}
	if output.Suggestion == "" {
		t.Fatal("expected suggestion for no results")
	}
}

func TestManager_CatalogNames(t *testing.T) {
	catalog := buildTestCatalog()
	searchTool := NewToolSearchTool(catalog)

	names := searchTool.CatalogNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	expected := map[string]bool{
		"weather_lookup": true,
		"translate_text": true,
		"file_read_file": true,
	}
	for _, name := range names {
		if !expected[name] {
			t.Fatalf("unexpected catalog name: %s", name)
		}
	}
}

func TestToolLoad_Success(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	loadTool := NewToolLoadToolWithManager(manager)

	ctx := withTestInvocation(context.Background())
	result, err := loadTool.Call(ctx, []byte(`{"tool_name": "weather_lookup"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolLoadOutput)
	if !ok {
		t.Fatalf("expected toolLoadOutput, got %T", result)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Schema == nil {
		t.Fatal("expected full schema in response")
	}
	if output.Schema.Name != "weather_lookup" {
		t.Fatalf("expected schema name weather_lookup, got %s", output.Schema.Name)
	}
	if output.Schema.Description != "Look up weather for a city" {
		t.Fatalf("unexpected schema description: %s", output.Schema.Description)
	}
	if !manager.IsActivated(ctx, "weather_lookup") {
		t.Fatal("weather_lookup should be activated after tool_load")
	}
}

func TestToolLoad_UnknownTool(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	loadTool := NewToolLoadToolWithManager(manager)

	ctx := withTestInvocation(context.Background())
	result, err := loadTool.Call(ctx, []byte(`{"tool_name": "no_such_tool"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := result.(toolLoadOutput)
	if output.Success {
		t.Fatal("expected failure for unknown tool")
	}
	if !strings.Contains(output.Error, "not found") {
		t.Fatalf("expected 'not found' error, got: %s", output.Error)
	}
}

func TestToolLoad_Idempotent(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	loadTool := NewToolLoadToolWithManager(manager)

	ctx := withTestInvocation(context.Background())
	for i := 0; i < 2; i++ {
		result, err := loadTool.Call(ctx, []byte(`{"tool_name": "weather_lookup"}`))
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
		if !result.(toolLoadOutput).Success {
			t.Fatalf("call %d should succeed", i)
		}
	}
}

func TestToolFilter_HidesBeforeActivation(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	filter := manager.ToolFilter()

	ctx := withTestInvocation(context.Background())
	weatherTool := createWeatherTool()

	if filter(ctx, weatherTool) {
		t.Fatal("weather_lookup should be filtered out before activation")
	}
}

func TestToolFilter_PassesAfterActivation(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	filter := manager.ToolFilter()

	ctx := withTestInvocation(context.Background())
	weatherTool := createWeatherTool()

	if _, err := manager.Activate(ctx, "weather_lookup"); err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if !filter(ctx, weatherTool) {
		t.Fatal("weather_lookup should pass filter after activation")
	}
}

func TestToolFilter_PassesNonDeferredTools(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	filter := manager.ToolFilter()

	nonDeferred := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("datetime"),
	)
	if !filter(context.Background(), nonDeferred) {
		t.Fatal("non-deferred tool should always pass filter")
	}
}

func TestToolFilter_HidesHostexecAliasByBaseName(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "hostexec_exec_command", BaseName: "exec_command", Description: "Run a shell command"},
	}
	manager := NewDeferredToolManager(catalog)
	filter := manager.ToolFilter()

	inner := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("exec_command"),
		trpcfunction.WithDescription("Run a shell command"),
	)
	deferred := NewDeferredCallableTool(inner, loggateway.NewNoop())
	manager.RegisterTool("hostexec_exec_command", deferred)
	alias := &fakeInnerWrapper{inner: deferred, name: "shell"}

	ctx := withTestInvocation(context.Background())
	if filter(ctx, alias) {
		t.Fatal("shell alias of deferred hostexec_exec_command must stay hidden until tool_load")
	}
	writeActivatedSet(ctx, "hostexec_exec_command")
	writeActivatedSet(ctx, "exec_command")
	if !filter(ctx, alias) {
		t.Fatal("shell alias should pass after activating hostexec_exec_command")
	}
}

func TestToolFilter_HidesMemoryAddUntilActivated(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "memory_add", BaseName: "memory_add", Description: "Add a memory"},
	}
	manager := NewDeferredToolManager(catalog)
	filter := manager.ToolFilter()

	inner := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("memory_add"),
		trpcfunction.WithDescription("Add a memory"),
	)
	wrapped := NewDeferredCallableTool(inner, loggateway.NewNoop())
	manager.RegisterTool("memory_add", wrapped)

	ctx := withTestInvocation(context.Background())
	if filter(ctx, wrapped) {
		t.Fatal("memory_add must stay hidden until tool_load")
	}
	writeActivatedSet(ctx, "memory_add")
	if !filter(ctx, wrapped) {
		t.Fatal("memory_add should pass after activation")
	}
}

func TestToolFilter_HidesWorkingMemoryWriteByBaseName(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "working_memory_write", BaseName: "write", Description: "Write a working-memory field"},
	}
	manager := NewDeferredToolManager(catalog)
	filter := manager.ToolFilter()

	inner := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("write"),
		trpcfunction.WithDescription("Write a working-memory field"),
	)
	deferred := NewDeferredCallableTool(inner, loggateway.NewNoop())
	manager.RegisterTool("working_memory_write", deferred)
	named := &fakeInnerWrapper{inner: deferred, name: "working_memory_write"}

	ctx := withTestInvocation(context.Background())
	if filter(ctx, named) {
		t.Fatal("working_memory_write must stay hidden until tool_load")
	}
	writeActivatedSet(ctx, "working_memory_write")
	writeActivatedSet(ctx, "write")
	if !filter(ctx, named) {
		t.Fatal("working_memory_write should pass after activating the catalog name")
	}
}

func TestToolFilter_PerSessionIsolation(t *testing.T) {
	catalog := buildTestCatalog()
	manager := newTestManager(catalog)
	filter := manager.ToolFilter()

	ctxA := withTestInvocation(context.Background())
	ctxB := withTestInvocation(context.Background())
	weatherTool := createWeatherTool()

	if _, err := manager.Activate(ctxA, "weather_lookup"); err != nil {
		t.Fatalf("activate failed: %v", err)
	}

	if !filter(ctxA, weatherTool) {
		t.Fatal("weather_lookup should be visible in session A after activation")
	}
	if filter(ctxB, weatherTool) {
		t.Fatal("weather_lookup must remain hidden in session B (per-session isolation)")
	}
}

// TestResolveDeferredName_PenetratesWrappers 验证 filter 的递归解包：
// alias/decorator 包装链下的延迟工具能被正确识别。
func TestResolveDeferredName_PenetratesWrappers(t *testing.T) {
	deferredNames := map[string]bool{"save_file": true}

	inner := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("save_file"),
	)

	// 单层 InnerTool 包装（模拟 aliasTool）
	wrapped := &fakeInnerWrapper{inner: inner, name: "write_file"}
	name, ok := resolveDeferredName(wrapped, deferredNames)
	if !ok || name != "save_file" {
		t.Fatalf("expected penetration to save_file, got %q ok=%v", name, ok)
	}

	// 多层包装：InnerTool → Original → 原始工具
	doubleWrapped := &fakeInnerWrapper{inner: &fakeOriginalWrapper{inner: inner}, name: "alias"}
	name, ok = resolveDeferredName(doubleWrapped, deferredNames)
	if !ok || name != "save_file" {
		t.Fatalf("expected multi-level penetration to save_file, got %q ok=%v", name, ok)
	}

	// 非延迟工具不命中
	other := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("datetime"),
	)
	if _, ok := resolveDeferredName(other, deferredNames); ok {
		t.Fatal("non-deferred tool should not resolve")
	}
}

type fakeInnerWrapper struct {
	inner trpctool.Tool
	name  string
}

func (w *fakeInnerWrapper) Declaration() *trpctool.Declaration {
	decl := w.inner.Declaration()
	clone := *decl
	clone.Name = w.name
	return &clone
}

func (w *fakeInnerWrapper) InnerTool() trpctool.Tool { return w.inner }

type fakeOriginalWrapper struct {
	inner trpctool.Tool
}

func (w *fakeOriginalWrapper) Declaration() *trpctool.Declaration {
	return w.inner.Declaration()
}

func (w *fakeOriginalWrapper) Original() trpctool.Tool { return w.inner }

func TestDeferredToolSet_WrapsDeferredTools(t *testing.T) {
	inner1 := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "1", nil },
		trpcfunction.WithName("read_file"),
	)
	inner2 := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "2", nil },
		trpcfunction.WithName("list_file"),
	)
	set := &fakeToolSet{name: "file", tools: []trpctool.Tool{inner1, inner2}}

	deferred := map[string]bool{"read_file": true}
	wrapped := NewDeferredToolSet(set, deferred, loggateway.NewNoop())

	tools := wrapped.Tools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// read_file 被包装为 DeferredCallableTool
	if _, ok := tools[0].(*DeferredCallableTool); !ok {
		t.Fatalf("read_file should be wrapped, got %T", tools[0])
	}
	// list_file 原样返回
	if _, ok := tools[1].(*DeferredCallableTool); ok {
		t.Fatal("list_file should NOT be wrapped")
	}
}

type fakeToolSet struct {
	name  string
	tools []trpctool.Tool
}

func (s *fakeToolSet) Name() string { return s.name }
func (s *fakeToolSet) Tools(_ context.Context) []trpctool.Tool {
	return s.tools
}
func (s *fakeToolSet) Close() error { return nil }
