package agent

import (
	"context"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	tooltrpc "aranea-agents/internal/tools/trpc"
)

// hot_swap.go — P0-2 阶段B：在线热替换（report-05 FR-2.5/FR-2.6）。
//
// 触发形态（对 report-05 的偏离登记，2026-08-16 评审批准）：惰性换面——
// MCP 配置变更使 MCPVersionHash 变化，下一请求算出新 key 走 miss 路径；
// 在 singleflight 内先找「兄弟 entry」（同 agent 旧 key 的存活构建）执行
// 原地换面，避免整实例重建（LLM agent 实例、core 等未变分片、回调链全部
// 保留）。任一门禁不通过即回退现有全量构建路径，语义零风险。
//
// 换面机制：用框架 llmagent.SetToolSets 整面原子替换（并发安全，自动
// refreshToolsLocked），而非 FR-2.5 字面的 Add/RemoveToolSet 按名 diff——
// 原子性更好、无部分失败窗口，且不再需要 shard:<group> 命名契约。flat 工具
// （option.Tools）框架无热替换 API，因此门禁③要求 flat 名集不变（MCP 配置
// 变更不影响 flat 面；broker 降级切换等改 flat 面的场景回退全量构建）。
//
// deferred 热替换（方案B 视图间接层，2026-08-16 用户裁定，取代原「deferred
// 硬边界」门禁）：DeferredToolManager 是稳定句柄，catalog/tools/names 收进
// 不可变 deferredView 经 atomic.Pointer 持有；filter 闭包、tool_search/
// tool_load 元工具、catalog cue hook 四件套全部绑定句柄、每次调用读当前
// 视图。换面时旧句柄 SwapView(新 manager) 原子切目录——deferred 集合变化
// （MCP 工具列表增删）亦可换面，新增延迟工具安装即被隐藏、移除名字不再
// 拦截；session 激活状态按名字自然延续。两侧 manager 对称性（同空/同非空）
// 由门禁③保证：catalog 非空 ⟺ flat 面含 tool_search/tool_load。
//
// 在途安全（FR-2.6）：热替换不换代——entry.handle 存活，旧分片 hold 组以
// 存活 handle 进 graveyard，sweeper 在 refs==0（无在途 run）的下一周期或
// 10min 兜底关闭 = 释放旧分片引用。在途 run 继续引用旧面工具对象（共享
// 分片产物本体由 shardCache 持有，绝不原地关闭），零 abort。

// faceShardMeta 是一个分片的面元数据（拓扑比对用）。
type faceShardMeta struct {
	id    string
	group string
	fp    string
}

// faceMeta 是 entry 持久化的热替换元数据。
type faceMeta struct {
	fp          buildKeyFP                   // 构建时的 key 指纹（MCPHash 为构建期旧值）
	shards      []faceShardMeta              // 与 plan.specs 同序
	deferredMgr *deferred.DeferredToolManager // 延迟工具稳定句柄（无延迟目录时 nil）
	flatNames   []string                     // 合并后 flat 工具名集（排序，去重）
}

// faceMetaFromPlan 从分片计划与合并产物提取面元数据。
func faceMetaFromPlan(fp buildKeyFP, sp *shardPlan, ts *tooltrpc.AssembledToolsets) *faceMeta {
	m := &faceMeta{fp: fp}
	if sp != nil {
		m.shards = make([]faceShardMeta, len(sp.specs))
		for i, spec := range sp.specs {
			m.shards[i] = faceShardMeta{id: spec.id, group: spec.group, fp: spec.fp}
		}
	}
	if ts != nil {
		m.deferredMgr = ts.DeferredManager
		m.flatNames = flatToolNames(ts.Tools)
	}
	return m
}

// flatToolNames 返回排序去重后的 flat 工具名集（声明名；与 customToolNames
// 不同，不含行为判别后缀——面比对只关心工具面构成）。
func flatToolNames(tools []trpctool.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		set[t.Declaration().Name] = true
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tryHotSwap 在 miss 路径尝试在线热替换。命中兄弟且三道门禁（拓扑/flat/
// setter 可用）全过时执行原地换面并返回存活 agent（entry 已 re-key 到
// newKey）；任何不适用情形返回 nil，调用方回退全量构建。
//
// 并发：仅由 singleflight 闭包调用（同 newKey 构建已去重）；items 读写经
// c.mu；构建与换面动作在锁外，提交时再校验 entry 未被替换/驱逐/标脏。
func (c *BuildCache) tryHotSwap(ctx context.Context, newKey string, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) trpcagent.Agent {
	newFP := computeBuildKeyFP(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, deps.MCPVersionHash)

	// 选兄弟：同 agentID 前缀、有面元数据、未脏、指纹仅 MCPHash 异。
	c.mu.Lock()
	var sibling *buildCacheEntry
	var siblingKey string
	for k, e := range c.items {
		if e.face == nil || e.dirty {
			continue
		}
		if e.face.fp.mcpOnlyDelta(newFP) {
			sibling, siblingKey = e, k
			c.lruList.MoveToFront(e.elem)
			break
		}
	}
	c.mu.Unlock()
	if sibling == nil {
		arametrics.AgentHotSwapTotal.WithLabelValues("no_sibling").Inc()
		return nil
	}
	oldFace := sibling.face

	// 构建新面（锁外；与整实例构建共用同一装配函数，语义天然一致）。
	plan, perr := loadToolBuildPlanForSwap(ctx, ag, deps)
	if perr != nil {
		lg.Warn("热替换计划加载失败，回退全量构建", loggateway.StepID("agent.hot_swap"), loggateway.Str("agent_id", ag.ID), loggateway.Err(perr))
		arametrics.AgentHotSwapTotal.WithLabelValues("plan_error").Inc()
		return nil
	}
	ts, holds, sp, buildErr := buildToolsetsForAgent(ctx, ag, deps, plan)
	if buildErr != nil {
		lg.Warn("热替换面构建失败，回退全量构建", loggateway.StepID("agent.hot_swap"), loggateway.Str("agent_id", ag.ID), loggateway.Err(buildErr))
		arametrics.AgentHotSwapTotal.WithLabelValues("build_error").Inc()
		return nil
	}

	// 门禁②：分片拓扑一致（等长同序、id/group 逐位等；fp 差异仅允许
	// mcp/mcp_broker 组——其他组 fp 变化说明变量不纯，回退）。
	if sp == nil || len(sp.specs) != len(oldFace.shards) {
		releaseHolds(holds)
		arametrics.AgentHotSwapTotal.WithLabelValues("topology_changed").Inc()
		return nil
	}
	for i, spec := range sp.specs {
		old := oldFace.shards[i]
		if spec.id != old.id || spec.group != old.group {
			releaseHolds(holds)
			arametrics.AgentHotSwapTotal.WithLabelValues("topology_changed").Inc()
			return nil
		}
		if spec.fp != old.fp && spec.group != shardGroupMCP && spec.group != shardGroupMCPBroker {
			releaseHolds(holds)
			arametrics.AgentHotSwapTotal.WithLabelValues("topology_changed").Inc()
			return nil
		}
	}

	// 门禁②：flat 工具名集不变（flat 无热替换 API）。同时保证 deferred
	// 两侧对称：catalog 非空 ⟺ flat 面含 tool_search/tool_load，非空↔空
	// 的不对称情形在此被拦（回退全量构建，由新构建安装/摘除四件套）。
	newFlat := flatToolNames(ts.Tools)
	if !equalStringSlices(newFlat, oldFace.flatNames) {
		releaseHolds(holds)
		arametrics.AgentHotSwapTotal.WithLabelValues("flat_changed").Inc()
		return nil
	}

	// 换面 + 提交。次序（fail-closed）：先 SwapView 切 deferred 目录（新
	// 延迟工具尚未安装即已被 filter 隐藏，杜绝「已安装未隐藏」泄露窗口），
	// 再 SetToolSets（锁外）：此后的 Run 立即见新工具；
	// 最后锁内校验 entry 仍为同一代际（未被 put 换代/驱逐/标脏/缓存关闭）。
	// sibling.agent 是 runScopedAgent 包装产物，其 SetToolSets 转发到内层
	// *llmagent.LLMAgent（cache_refcount.go）；接口断言兜底非包装路径。
	swapStart := time.Now()
	viewSwapped := false
	if oldFace.deferredMgr != nil && ts.DeferredManager != nil {
		oldFace.deferredMgr.SwapView(ts.DeferredManager)
		viewSwapped = true
	}
	setter, ok := sibling.agent.(interface{ SetToolSets([]trpctool.ToolSet) })
	if !ok {
		releaseHolds(holds)
		arametrics.AgentHotSwapTotal.WithLabelValues("no_setter").Inc()
		return nil
	}
	setter.SetToolSets(ts.ToolSets)

	c.mu.Lock()
	if c.closed || c.items[siblingKey] != sibling || sibling.agent == nil {
		c.mu.Unlock()
		// entry 已被置换/驱逐：SwapView/SetToolSets 作用于孤儿 agent（无
		// 害，无人再服务）；新面引用关闭释放，回退全量构建。
		releaseHolds(holds)
		arametrics.AgentHotSwapTotal.WithLabelValues("commit_conflict").Inc()
		return nil
	}
	oldHolds := sibling.toolSets
	sibling.toolSets = holds
	sibling.face = &faceMeta{
		fp:          newFP,
		shards:      shardMetasOf(sp),
		deferredMgr: oldFace.deferredMgr, // 稳定句柄存活：视图已切，四件套持续生效
		flatNames:   newFlat,
	}
	delete(c.items, siblingKey)
	sibling.key = newKey
	sibling.dirty = false
	c.items[newKey] = sibling
	handle := sibling.handle
	c.mu.Unlock()

	// 旧 hold 组以存活 handle 进 graveyard：refs==0（在途排空）次周期关，
	// 或 10min 兜底（FR-2.6）。
	c.mu.Lock()
	c.retireToolSets(oldHolds, newKey, handle)
	c.mu.Unlock()

	swapMs := time.Since(swapStart).Milliseconds()
	arametrics.AgentHotSwapTotal.WithLabelValues("applied").Inc()
	lg.Info("Agent 在线热替换完成（免整实例重建）",
		loggateway.StepID("agent.hot_swap"),
		loggateway.Str("agent_id", ag.ID),
		loggateway.Str("agent_key", ag.AgentKey),
		loggateway.Int64("swap_ms", swapMs),
		loggateway.Int("shards", len(sp.specs)),
		loggateway.Bool("deferred_view_swapped", viewSwapped))
	c.emitCacheFlow(ctx, lg, "system.agent.hot_swap", "Agent 在线热替换完成（免整实例重建）",
		event.P("agent_id", ag.ID), event.P("agent_key", ag.AgentKey), event.P("swap_ms", swapMs), event.P("deferred_view_swapped", viewSwapped))
	return sibling.agent
}

// shardMetasOf 提取 plan 的分片面元数据。
func shardMetasOf(sp *shardPlan) []faceShardMeta {
	if sp == nil {
		return nil
	}
	out := make([]faceShardMeta, len(sp.specs))
	for i, spec := range sp.specs {
		out[i] = faceShardMeta{id: spec.id, group: spec.group, fp: spec.fp}
	}
	return out
}

// releaseHolds 关闭未安装的 hold 单元（中止路径）：Close 占位符 = 释放
// 分片引用，与 entry 退役语义一致。
func releaseHolds(holds []trpctool.ToolSet) {
	for _, h := range holds {
		if h != nil {
			_ = h.Close()
		}
	}
}

// loadToolBuildPlanForSwap 为热替换加载构建计划（eff/目录快照/确认门），
// 与 buildTRPCLLMAgentWithToolSets 的加载步骤一致（行为等价要求）。
func loadToolBuildPlanForSwap(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*toolBuildPlan, error) {
	var eff map[string]bool
	var catalog *toolBuildCatalog
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff = loadEffectiveToolKeys(ctx, deps, ag.ID)
		catalog = loadToolBuildCatalog(ctx, ag.ID, eff, deps)
	}
	gate := buildToolConfirmGate(ctx, ag, deps, catalog.confirmCatalog(eff))
	return &toolBuildPlan{eff: eff, catalog: catalog, gate: gate}, nil
}
