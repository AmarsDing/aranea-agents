package toolapi

import (
	"context"

	"google.golang.org/adk/tool"
)

// Tool 是项目内的统一抽象：同时具备「进程内可调」与可选的 ADK/OpenAI 侧声明。
//
// SupportsLocalInvoke 为 true 时，Runner 不要求即可通过 Registry.Invoke 执行；
// Gemini 后端专用或依赖 tool.Context 状态机的工具常为 false。
type Tool interface {
	Meta() Meta

	// SupportsLocalInvoke 为 true 时 InvokeLocal 必须可用且不依赖 ADK InvocationContext。
	SupportsLocalInvoke() bool

	InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error)

	// OpenAIFunction 返回整条 OpenAI-compat tools[] 条目；若 nil 则由调用方不向 OpenAI 暴露该函数。
	OpenAIFunction() map[string]any

	// ADKTool 返回 ADK Runner 侧的 tool.Tool；若本工具不适用 ADK 则 (nil, nil)。
	ADKTool() (tool.Tool, error)
}
