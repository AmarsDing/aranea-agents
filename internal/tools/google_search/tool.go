// Package google_search 暴露 Gemini 内置 Google Search 工具声明（需对应 genai 后端支持）。
package google_search

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
)

type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	gs := geminitool.GoogleSearch{}
	return toolapi.Meta{
		Name:        gs.Name(),
		TitleZh:     "谷歌搜索（Gemini 内置）",
		SummaryZh:   "由 Gemini 服务端在支持的模型上调用的 Google Search 能力（本地 OpenAI-compat 链路不会重复实现搜索）。",
		Description: gs.Description(),
	}
}

func (*impl) SupportsLocalInvoke() bool { return false }

func (*impl) InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error) {
	_ = ctx
	_ = args
	return nil, fmt.Errorf("%s 需在支持 genai.GoogleSearch 的后端上调度", (&impl{}).Meta().Name)
}

func (*impl) OpenAIFunction() map[string]any {
	return nil
}

func (*impl) ADKTool() (tool.Tool, error) {
	return geminitool.GoogleSearch{}, nil
}
