package agent

import (
	"context"

	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// shard_merge.go — P0-2 阶段A：分片获取与合并。
//
// 合并期重放全部横切处理（对跨分片并集统一执行，语义对齐单体装配期）：
//
//	1. MCP schema 治理（≡ 装配期 phase 7 治理分支）：直连分片 toolset 并集
//	   截断+总预算；超预算降级 broker（broker 分片产物已在并集中，或用
//	   BrokerFallback 现场构建）。共享产物绝不 Close——降级时释放的是
//	   分片引用（refs--），产物本体由分片缓存拥有。
//	2. 延迟工具目录+包装（≡ phase 10，FinalizeDeferredTools）。
//	3. flat 名去重（≡ phase 11，DedupFlatToolNames，earlier-wins 跨分片）。
//	4. 消歧提示（≡ phase 11，ApplyMergeDisambiguationHints：flat 就地包装
//	   并集切片；toolset 成员经 copy-on-write 包装，不改写共享产物）。
//	5. 运行时别名（≡ trpc 包装器，ApplyRuntimeNameAliases）。
//
// 确认门与装饰器不在此处——它们只依赖并集结果，由 buildToolsetsForAgent
// 尾部按原样重放（策略类字段变更因此只需重放，不触发任何分片重建）。

// acquireShardPlan 按确定性 spec 序获取全部计划分片。任一获取失败即释放
// 已获取的引用并返回错误（等价单体装配的 closeAll 错误路径）。
// 返回与 plan.specs 等长的产物/释放切片；释放函数幂等。
func acquireShardPlan(ctx context.Context, cache *shardCache, plan *shardPlan) ([]*shardProduct, []func(), error) {
	prods := make([]*shardProduct, len(plan.specs))
	releases := make([]func(), len(plan.specs))
	for i, spec := range plan.specs {
		prod, release, err := cache.acquire(ctx, spec)
		if err != nil {
			releaseShardRefs(releases[:i])
			return nil, nil, err
		}
		prods[i], releases[i] = prod, release
	}
	return prods, releases, nil
}

// releaseShardRefs 释放全部非空引用（幂等安全，可重复调用）。
func releaseShardRefs(releases []func()) {
	for _, r := range releases {
		if r != nil {
			r()
		}
	}
}

// mergeShardProducts 合并分片产物并重放横切处理。失败路径释放全部未消费
// 引用（成功路径的引用由调用方包装为 shardHoldToolSet 进入 retire 单元）。
func mergeShardProducts(ctx context.Context, sp *shardPlan, prods []*shardProduct, releases []func(), lg loggateway.Logger) (*tools.AssembledToolsets, error) {
	// 1. 直连 MCP 治理（跨 server 并集决策）。
	var governedDirect []trpctool.ToolSet
	var brokerFallbackTools []trpctool.Tool
	if len(sp.mcpIdx) > 0 {
		var direct []trpctool.ToolSet
		for _, idx := range sp.mcpIdx {
			direct = append(direct, prods[idx].toolSets...)
		}
		gov := tools.GovernMCPServerToolSetsAt(ctx, direct, lg, mcpDegradeToolCount(sp))
		if gov.TruncatedCount > 0 {
			lg.Info("MCP schema 治理：截断超长 declaration",
				loggateway.Domain("tools.mcp"),
				loggateway.Int("tool_count", gov.ToolCount),
				loggateway.Int("truncated_count", gov.TruncatedCount),
				loggateway.Int("total_chars", gov.TotalChars))
		}
		switch {
		case !gov.Degraded:
			governedDirect = gov.Kept
		case sp.brokerIdx >= 0 || sp.brokerFallback != nil:
			// 降级：直连分片不进入工具面，引用立即释放（产物本体由分片
			// 缓存拥有，refs 归零后由 LRU 淘汰关闭，语义等价装配期的
			// Close Kept 释放池引用）。broker 工具来自 broker 分片产物；
			// 无 broker 分片时用 fallback 现场构建（P1-2 降级语义）。
			for _, idx := range sp.mcpIdx {
				if releases[idx] != nil {
					releases[idx]()
					releases[idx] = nil
				}
			}
			if sp.brokerIdx < 0 {
				bt, err := tools.BuildMCPBrokerTools(*sp.brokerFallback)
				if err != nil {
					releaseShardRefs(releases)
					return nil, apierror.Internal(apierror.DomainTool, "mcpbroker: "+err.Error())
				}
				brokerFallbackTools = bt
			}
			lg.Warn("MCP schema 总量超预算，直连模式降级为 broker",
				loggateway.Domain("tools.mcp"),
				loggateway.Int("tool_count", gov.ToolCount),
				loggateway.Int("total_chars", gov.TotalChars),
				loggateway.Int("budget_chars", tools.MCPSchemaTotalBudgetChars()))
		default:
			// 无 broker 可降级：保留截断后的直连工具（best-effort）。
			governedDirect = gov.Kept
			lg.Warn("MCP schema 总量超预算且无 broker 配置，保留截断后的直连工具",
				loggateway.Domain("tools.mcp"),
				loggateway.Int("tool_count", gov.ToolCount),
				loggateway.Int("total_chars", gov.TotalChars),
				loggateway.Int("budget_chars", tools.MCPSchemaTotalBudgetChars()))
		}
	}

	// 2. 并集组装（确定性 spec 序；治理结果在首个直连 MCP 分片位插入，
	// 保持与单体装配「MCP 位于核心 toolset 之后、会话类 flat 工具之前」
	// 的相对区位）。
	out := &tools.AssembledToolsets{}
	mcpPos := make(map[int]bool, len(sp.mcpIdx))
	for _, idx := range sp.mcpIdx {
		mcpPos[idx] = true
	}
	governedInserted := false
	for i, prod := range prods {
		if mcpPos[i] {
			if !governedInserted {
				out.ToolSets = append(out.ToolSets, governedDirect...)
				out.Tools = append(out.Tools, brokerFallbackTools...)
				governedInserted = true
			}
			continue
		}
		out.ToolSets = append(out.ToolSets, prod.toolSets...)
		out.Tools = append(out.Tools, prod.tools...)
	}

	// 3. 延迟工具目录+包装（≡ phase 10）。
	if len(sp.deferredTools) > 0 {
		if err := tools.FinalizeDeferredTools(ctx, out, sp.deferredTools, lg); err != nil {
			releaseShardRefs(releases)
			return nil, err
		}
	}

	// 4-5. 去重 + 消歧 + 别名（≡ phase 11 + trpc 包装器，共享安全形态）。
	tools.DedupFlatToolNames(ctx, out, lg)
	tools.ApplyMergeDisambiguationHints(out)
	tools.ApplyRuntimeNameAliases(ctx, out)
	return out, nil
}

func mcpDegradeToolCount(sp *shardPlan) int {
	if sp == nil {
		return 0
	}
	maxTools := tools.MCPSchemaToolCountDegradeForProfile(sp.toolsProfile)
	if !sp.mcpAllowExplicit && tools.MCPSchemaPreferBrokerWithoutAllow(sp.toolsProfile) {
		return 1
	}
	return maxTools
}
