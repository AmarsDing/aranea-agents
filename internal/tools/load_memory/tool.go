// Package load_memory 支持模型按 query 拉取用户记忆片段（依赖 ADK MemoryService）。
package load_memory

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/loadmemorytool"
)

// impl 让模型按需根据 query 拉取与用户相关的记忆条目（底层走 MemoryService.SearchMemory）。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	tk := loadmemorytool.New()
	return toolapi.Meta{
		Name:        tk.Name(),
		TitleZh:     "按需加载记忆",
		SummaryZh:   "当用户提问需要回忆历史信息时可调用（通过 ADK 记忆后端检索语义相关片段）。",
		Description: tk.Description(),
	}
}

func (*impl) SupportsLocalInvoke() bool { return false }

func (*impl) InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error) {
	_ = ctx
	_ = args
	return nil, fmt.Errorf("%s 仅 ADK Runner 内可调", loadmemorytool.New().Name())
}

func (*impl) OpenAIFunction() map[string]any {
	return nil
}

func (*impl) ADKTool() (tool.Tool, error) {
	return loadmemorytool.New(), nil
}
