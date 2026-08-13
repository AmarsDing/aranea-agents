package agent

import (
	"context"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools/deferred"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// newToolCatalogCueBeforeHook 创建静态目录 cue 的 BeforeModel hook。
//
// 当 agent 有 deferred 工具时，将「工具名 + 一句话描述」清单追加到消息流末尾，
// 让模型知道有哪些按需加载的工具可用。cue 内容在 agent 构建时确定（静态），
// 会话内不变，保持缓存前缀稳定。
//
// 设计依据（29-token §14.4 WP-4）：
//   - 静态目录 cue：长尾工具以「工具名 + 一句话描述」清单注入
//   - 按 key 排序、无动态状态
//   - 模型经 tool_load 元工具按需把完整 schema 加载进消息流尾部
func newToolCatalogCueBeforeHook(deps TRPCBuilderDeps) callbacks.Callback {
	dm := deps.DeferredManager
	if dm == nil {
		return nil
	}
	catalog := dm.Catalog()
	if len(catalog) == 0 {
		return nil
	}
	cue := deferred.RenderCatalogCue(catalog)
	if cue == "" {
		return nil
	}

	return callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 上下文预算台账（29-token §9.6）：目录 cue 计量。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryToolsSchema, utf8.RuneCountInString(cue))
		// 追加到消息末尾（非系统 prompt），保持前缀缓存稳定。
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
