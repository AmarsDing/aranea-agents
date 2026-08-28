package deferred

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/metrics"

	"trpc.group/trpc-go/trpc-agent-go/toolsnapshot"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type toolLoadInput struct {
	ToolName string `json:"tool_name" jsonschema:"description=Name of the tool to load and activate,required"`
}

type toolLoadOutput struct {
	Success     bool                  `json:"success"`
	ToolName    string                `json:"tool_name,omitempty"`
	Description string                `json:"description,omitempty"`
	Schema      *trpctool.Declaration `json:"schema,omitempty"`
	Error       string                `json:"error,omitempty"`
	Message     string                `json:"message,omitempty"`
}

// ToolLoadTool 是一个元工具，允许模型按名称直接加载和激活延迟工具。
//
// 与 tool_search 的区别：
//   - tool_search：模型不知道有什么工具 → 搜索发现 → 返回匹配列表
//   - tool_load：模型已从目录 cue 知道工具名 → 直接激活 → 立即可用
//
// 设计依据（29-token §14.4 WP-4）：
// 静态目录 cue 列出所有延迟工具的名称+描述，模型通过 tool_load 按需加载完整 schema。
// 激活后写入 session state 并触发 LLM 工具快照失效，本轮下一次模型请求
// （非同批并行 tool call）即能看到新工具。
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
		trpcfunction.WithDescription("Load and activate a deferred tool by name. Use this when you know which tool you need from the available tools catalog. After this call returns, you can use the tool on the next model call in this turn — not in the same parallel tool batch."),
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
		trpcfunction.WithDescription("Load and activate a deferred tool by name. Use this when you know which tool you need from the available tools catalog. After this call returns, you can use the tool on the next model call in this turn — not in the same parallel tool batch."),
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

	// 名称解析：目录运行时名精确匹配 → 唯一基础名 → 别名链（shell_exec →
	// exec_command → hostexec_exec_command）。模型只知变体名时也能激活正确工具，
	// 避免「not found → 臆造名重试」空转。
	canonical, ok := t.manager.ResolveName(in.ToolName)
	if !ok {
		// P1-4 漏斗度量（激活段）：未找到目标工具。
		metrics.DeferredToolActivationTotal.WithLabelValues(in.ToolName, "not_found").Inc()
		return toolLoadOutput{
			Success:  false,
			ToolName: in.ToolName,
			Error: fmt.Sprintf("tool %q not found in deferred catalog. Available deferred tools (%d): %s. "+
				"Retry tool_load with one of these exact names, or use tool_search to discover tools by capability.",
				in.ToolName, len(t.manager.CatalogNames()), strings.Join(t.manager.CatalogNames(), ", ")),
		}, nil
	}

	// 激活工具（幂等：已激活则直接返回声明）
	decl, err := t.manager.Activate(ctx, canonical)
	if err != nil {
		metrics.DeferredToolActivationTotal.WithLabelValues(canonical, "failed").Inc()
		return toolLoadOutput{
			Success:  false,
			ToolName: canonical,
			Error:    fmt.Sprintf("failed to activate tool %q: %v", canonical, err),
		}, nil
	}
	metrics.DeferredToolActivationTotal.WithLabelValues(canonical, "success").Inc()

	// 触发 LLM 工具快照失效，下一轮请求即能看到新工具
	toolsnapshot.InvalidateFromContext(ctx)

	// 构建结果：返回完整声明供模型立即了解 schema
	desc := ""
	if decl != nil {
		desc = decl.Description
	}

	msg := fmt.Sprintf("Tool %q loaded and activated successfully. Call %q on the next model step in this turn; do not call it in the same parallel tool batch as this tool_load.", canonical, canonical)
	if canonical != in.ToolName {
		msg = fmt.Sprintf("Tool %q resolved to %q and activated successfully. You must call %q (not %q) on the next model step in this turn; do not call it in the same parallel tool batch as this tool_load.", in.ToolName, canonical, canonical, in.ToolName)
	}

	return toolLoadOutput{
		Success:     true,
		ToolName:    canonical,
		Description: desc,
		Schema:      decl,
		Message:     msg,
	}, nil
}
