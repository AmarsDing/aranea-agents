package deferred

import (
	"context"

	"aranea-agents/pkg/loggateway"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// DeferredToolSet 包装一个 ToolSet，在每次 Tools(ctx) 调用时将延迟工具
// 包装为 DeferredCallableTool。非延迟工具原样返回。
//
// 这样延迟工具在 LLM tools block 中被 ToolFilter 隐藏（0 schema token），
// 激活后通过 filter 放行并展示完整 schema。
type DeferredToolSet struct {
	inner    trpctool.ToolSet
	deferred map[string]bool // 延迟工具名称集合
	lg       loggateway.Logger
}

// NewDeferredToolSet 创建一个延迟 ToolSet 包装器。
// deferred 参数包含需要延迟的工具名称（Declaration().Name）。
func NewDeferredToolSet(inner trpctool.ToolSet, deferred map[string]bool, lg loggateway.Logger) *DeferredToolSet {
	return &DeferredToolSet{
		inner:    inner,
		deferred: deferred,
		lg:       lg,
	}
}

func (s *DeferredToolSet) Name() string {
	return s.inner.Name()
}

func (s *DeferredToolSet) Tools(ctx context.Context) []trpctool.Tool {
	tools := s.inner.Tools(ctx)
	out := make([]trpctool.Tool, len(tools))
	for i, t := range tools {
		if t == nil || t.Declaration() == nil {
			out[i] = t
			continue
		}
		if s.deferred[t.Declaration().Name] {
			out[i] = NewDeferredCallableTool(t, s.lg)
		} else {
			out[i] = t
		}
	}
	return out
}

func (s *DeferredToolSet) Close() error {
	return s.inner.Close()
}
