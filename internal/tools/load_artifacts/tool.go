// Package load_artifacts 将会话工件暴露给模型并按需拉取内容（依赖 ADK Artifact 服务）。
package load_artifacts

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/loadartifactstool"
)

// impl 将可用工件加载进会话并让模型可选用（依赖 ADK Session/Artifact）。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	tk := loadartifactstool.New()
	return toolapi.Meta{
		Name:        tk.Name(),
		TitleZh:     "加载工件",
		SummaryZh:   "把工作区会话中的工件载入模型上下文清单，按需通过函数调用取回内容（需 ADK Artifact 服务）。",
		Description: tk.Description(),
	}
}

func (*impl) SupportsLocalInvoke() bool { return false }

func (*impl) InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error) {
	_ = ctx
	_ = args
	return nil, fmt.Errorf("%s 仅 ADK Runner 内的工具回调可执行", loadartifactstool.New().Name())
}

func (*impl) OpenAIFunction() map[string]any {
	return nil
}

func (*impl) ADKTool() (tool.Tool, error) {
	return loadartifactstool.New(), nil
}
