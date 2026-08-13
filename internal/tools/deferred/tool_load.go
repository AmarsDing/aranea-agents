package deferred

import (
	"context"
	"fmt"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type toolLoadInput struct {
	ToolName string `json:"tool_name" jsonschema:"description=Name of the tool to load and activate,required"`
}

type toolLoadOutput struct {
	Success  bool   `json:"success"`
	ToolName string `json:"tool_name,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ToolLoadTool 是一个元工具，允许模型按名称直接加载和激活延迟工具。
//
// 与 tool_search 的区别：
//   - tool_search：模型不知道有什么工具 → 搜索发现 → Discover 标记 → 下轮放行
//   - tool_load：模型已从目录 cue 知道工具名 → 直接激活 → 立即可用
//
// 设计依据（29-token §14.4 WP-4）：
// 静态目录 cue 列出所有延迟工具的名称+描述，模型通过 tool_load 按需加载完整 schema。
type ToolLoadTool struct {
	tool    trpctool.CallableTool
	manager *DeferredToolManager
}

// NewToolLoadTool 创建 tool_load 元工具。
// catalog 必须与 ToolSearchTool 共享同一个 DeferredToolManager，
// 以便 tool_search 发现的工具和 tool_load 激活的工具状态一致。
func NewToolLoadTool(catalog []DeferredToolEntry) *ToolLoadTool {
	manager := NewDeferredToolManager(catalog)
	t := &ToolLoadTool{manager: manager}
	t.tool = trpcfunction.NewFunctionTool(
		t.execute,
		trpcfunction.WithName("tool_load"),
		trpcfunction.WithDescription("Load and activate a deferred tool by name. Use this when you know which tool you need from the available tools catalog. The tool will be immediately available for use in subsequent requests."),
	)
	return t
}

// NewToolLoadToolWithManager 创建与现有 manager 共享状态的 tool_load 元工具。
// 当 ToolSearchTool 已经创建了 manager 时，使用此方法确保状态一致。
func NewToolLoadToolWithManager(manager *DeferredToolManager) *ToolLoadTool {
	t := &ToolLoadTool{manager: manager}
	t.tool = trpcfunction.NewFunctionTool(
		t.execute,
		trpcfunction.WithName("tool_load"),
		trpcfunction.WithDescription("Load and activate a deferred tool by name. Use this when you know which tool you need from the available tools catalog. The tool will be immediately available for use in subsequent requests."),
	)
	return t
}

func (t *ToolLoadTool) Manager() *DeferredToolManager {
	return t.manager
}

func (t *ToolLoadTool) Declaration() *trpctool.Declaration {
	return t.tool.Declaration()
}

func (t *ToolLoadTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	return t.tool.Call(ctx, jsonArgs)
}

func (t *ToolLoadTool) execute(ctx context.Context, in toolLoadInput) (toolLoadOutput, error) {
	if in.ToolName == "" {
		return toolLoadOutput{
			Success: false,
			Error:   "tool_name is required",
		}, nil
	}

	// 检查工具是否在目录中
	if !t.manager.IsInCatalog(in.ToolName) {
		return toolLoadOutput{
			Success: false,
			ToolName: in.ToolName,
			Error:   fmt.Sprintf("tool %q not found in deferred catalog", in.ToolName),
		}, nil
	}

	// 激活工具（幂等：已激活则直接返回缓存）
	activatedTool, err := t.manager.Activate(ctx, in.ToolName)
	if err != nil {
		return toolLoadOutput{
			Success:  false,
			ToolName: in.ToolName,
			Error:    fmt.Sprintf("failed to activate tool %q: %v", in.ToolName, err),
		}, nil
	}

	// 同时标记为已发现（确保 ToolFilter 放行）
	t.manager.Discover(in.ToolName)

	decl := activatedTool.Declaration()
	desc := ""
	if decl != nil {
		desc = decl.Description
	}

	return toolLoadOutput{
		Success:  true,
		ToolName: in.ToolName,
		Message:  fmt.Sprintf("Tool %q loaded successfully. %s", in.ToolName, desc),
	}, nil
}
