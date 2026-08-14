package agent

import (
	"context"
	"strconv"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/tools/deferred"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// newToolCatalogCueBeforeHook 创建目录 cue 的 BeforeModel hook。
//
// 当 agent 有 deferred 工具时，将「工具名 + 一句话描述」清单追加到消息流末尾，
// 让模型知道有哪些按需加载的工具可用。目录区按 key 排序、无动态状态；
// P1-4 起每轮按当前用户 query 追加 Top-N「推荐区」（语义预激活），
// 无匹配时输出与静态版字节一致。
//
// 设计依据（29-token §14.4 WP-4 + P1-4）：
//   - 静态目录区：长尾工具以「工具名 + 一句话描述」清单注入，按 key 排序
//   - 动态推荐区：按 query 相关度排序 Top-N，提升模型发现率
//   - cue 注入在消息流尾部（最后一条用户消息之后），本就处于可缓存前缀之外，
//     每轮动态渲染对前缀缓存零影响
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
	staticCue := deferred.RenderCatalogCue(catalog)
	if staticCue == "" {
		return nil
	}

	return callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// P1-4 语义预激活：按当前用户 query 渲染 Top-N 推荐区。
		cue := staticCue
		matched := false
		query := lastUserMessageText(args.Request.Messages)
		if recommended := deferred.RankCatalogEntries(catalog, query, deferred.CatalogRecommendLimit); len(recommended) > 0 {
			cue = deferred.RenderCatalogCueWithRecommendations(catalog, recommended)
			matched = true
		}
		// P1-4 漏斗度量：预激活覆盖率（推荐区非空的轮次占比）。
		metrics.DeferredCatalogRecommendTotal.WithLabelValues(strconv.FormatBool(matched)).Inc()
		// 上下文预算台账（29-token §9.6）：目录 cue 计量。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryToolsSchema, utf8.RuneCountInString(cue))
		// 追加到消息末尾（非系统 prompt），保持前缀缓存稳定。
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
