// Package preload_memory 在每次 LLM 请求前预取相关记忆并写入系统提示（依赖 ADK 记忆检索）。
package preload_memory

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/preloadmemorytool"
)

// impl 在每次发往 LLM 之前预取记忆并写入系统片段（不改变模型可调函数集，但被 Registry 归为 ADK 侧工具）。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	tk := preloadmemorytool.New()
	return toolapi.Meta{
		Name:        tk.Name(),
		TitleZh:     "预加载记忆上下文",
		SummaryZh:   "在用户消息进入模型前嵌入相关历史会话摘要（需 ADK 记忆后端与 Runner 的请求管线）。",
		Description: tk.Description(),
	}
}

func (*impl) SupportsLocalInvoke() bool { return false }

func (*impl) InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error) {
	_ = ctx
	_ = args
	return nil, fmt.Errorf("%s 仅在 ADK 请求预处理阶段生效", preloadmemorytool.New().Name())
}

func (*impl) OpenAIFunction() map[string]any {
	return nil
}

func (*impl) ADKTool() (tool.Tool, error) {
	return preloadmemorytool.New(), nil
}
