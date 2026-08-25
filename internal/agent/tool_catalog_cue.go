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
// P0-2B 方案B：目录与静态 cue 每轮经 CatalogSnapshot 读当前视图（原子加载），
// 热替换 SwapView 后下一轮即渲染新目录，hook 自身无需替换。
//
// 设计依据（29-token §14.4 WP-4 + P1-4）：
//   - 静态目录区：长尾工具以「工具名 + 一句话描述」清单注入，按 key 排序
//   - 动态推荐区：按 query 相关度排序 Top-N，提升模型发现率
//   - cue 注入在消息流尾部（最后一条用户消息之后），本就处于可缓存前缀之外，
//     每轮动态渲染对前缀缓存零影响
//   - 模型经 tool_load 元工具按需把完整 schema 加载进消息流尾部
//
// 分区说明（79-runtime-governance 附录 A · F-A5 整改 2026-08-25）：本 hook 标
// LayerSemiStatic 仅控执行序（先于 LayerDynamic 的注入类 hook 执行），不代表
// 内容会话内稳定——推荐区按当前 query 每轮可变。落区恒为 tail（appendDynamicCue），
// 不触碰 head/conv，与 Cache-First 装配契约（C1/C2）无冲突；行为不变，仅澄清语义。
func newToolCatalogCueBeforeHook(deps TRPCBuilderDeps) callbacks.Callback {
	dm := deps.DeferredManager
	if dm == nil {
		return nil
	}
	if len(dm.CatalogNames()) == 0 {
		return nil
	}

	return callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		catalog, staticCue := dm.CatalogSnapshot()
		if staticCue == "" {
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
		// 上下文预算台账（29-token §9.6）：目录 cue 与 Request.Tools schema 分列，
		// 避免先写入 tools_schema 导致计量 hook 跳过、tools_count 恒为 0。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryToolCatalogCue, utf8.RuneCountInString(cue))
		args.Request.Messages = appendDynamicCue(args.Request.Messages, toolCatalogCueMarker+cue)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
