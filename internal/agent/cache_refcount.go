package agent

import (
	"context"
	"sync/atomic"
	"time"

	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// P0-4 精确 in-flight 回收（Cordis D7/A3 的业务层形态）。
//
// 设计（实现期对 report-05 FR-4.1 的偏离，理由与等价性论证见 report-05
// P0-4 落地状态）：评审方案为「获取返回 (agent, release) + 调用点 run 终态
// defer release」。勘察发现 8 个获取点生命周期高度异质——graph resolver /
// runner AgentFactory / agent-as-tool 的 run 终态藏在框架内部，无挂点；
// 显式 release 在这些路径只能靠约定防遗漏（正是评审 R2 的缓解对象）。
// 因此把 acquire/release 下沉为 Run 作用域：缓存存取的是 runScopedAgent
// 包装产物，Run 开始即 acquire、事件流终结（含出错/消费者放弃）即 release。
// 语义等价（在途 run 持有代际引用）且从构造上消除 Release 遗漏；全部
// 获取点零改动。已知边界：「取出 agent 但迟迟不 Run」不在保护范围——
// 生产全部路径 build 后立即 Run（ms 级），远小于 sweeper 周期（1min）。
//
// 代际句柄：entry 每次 put 换代生成新 handle；旧代际 toolSets 进 graveyard
// 时携带旧 handle。sweeper 关闭条件从「age≥10min」改为
// 「refs==0（下一个清扫周期即关） || age≥retireDelay（兜底，refs>0 即泄漏，
// 打 Warn + 指标）」。Close（进程退出）语义不变：无视计数立即关。

// agentHandle 是一代构建产物的句柄：包装后的 agent + 在途 run 计数。
type agentHandle struct {
	agent trpcagent.Agent // runScopedAgent 包装产物
	refs  atomic.Int64
}

func newAgentHandle(inner trpcagent.Agent) *agentHandle {
	h := &agentHandle{}
	h.agent = &runScopedAgent{inner: inner, handle: h}
	return h
}

func (h *agentHandle) acquire() {
	h.refs.Add(1)
	arametrics.AgentCacheActiveRefs.Inc()
}

func (h *agentHandle) release() {
	h.refs.Add(-1)
	arametrics.AgentCacheActiveRefs.Dec()
}

func (h *agentHandle) load() int64 {
	if h == nil {
		return 0
	}
	return h.refs.Load()
}

// runScopedAgent 包装缓存 agent：Run 期间持有本代际引用，防止在途调用的
// ToolSets（MCP 会话/stdio 子进程）被 sweeper 提前关闭。转发方法与
// summaryFallbackAgent 先例一致，另转发 team 框架类型断言依赖的可选能力
// （SubAgentSetter / AddToolSet）与 P0-2B 在线热替换将使用的
// RemoveToolSet/SetToolSets——缓存内层恒为 *llmagent.LLMAgent，断言必成功；
// 测试桩不支持时降级 no-op（不产生误判）。
type runScopedAgent struct {
	inner  trpcagent.Agent
	handle *agentHandle
}

func (a *runScopedAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	a.handle.acquire()
	ch, err := a.inner.Run(ctx, inv)
	if err != nil {
		a.handle.release()
		return nil, err
	}
	out := make(chan *trpcevent.Event, 256)
	safego.Go(ctx, "agent.cache.run_scoped_pump", func() { a.pump(ctx, inv, ch, out) })
	return out, nil
}

// pump 转发事件流并在终结时释放引用。EmitEvent 失败（ctx 取消/消费者放弃）
// 直接返回——内层生产者随 ctx 收敛，与 summaryFallbackAgent.pump 同范式。
func (a *runScopedAgent) pump(ctx context.Context, inv *trpcagent.Invocation, in <-chan *trpcevent.Event, out chan<- *trpcevent.Event) {
	defer close(out)
	defer a.handle.release()
	for ev := range in {
		if err := trpcagent.EmitEvent(ctx, inv, out, ev); err != nil {
			return
		}
	}
}

func (a *runScopedAgent) Tools() []trpctool.Tool { return a.inner.Tools() }

func (a *runScopedAgent) Info() trpcagent.Info { return a.inner.Info() }

func (a *runScopedAgent) SubAgents() []trpcagent.Agent { return a.inner.SubAgents() }

func (a *runScopedAgent) FindSubAgent(name string) trpcagent.Agent {
	return a.inner.FindSubAgent(name)
}

// SetSubAgents 转发 agent.SubAgentSetter（team swarm 成员装配断言此接口）。
func (a *runScopedAgent) SetSubAgents(subAgents []trpcagent.Agent) {
	if s, ok := a.inner.(trpcagent.SubAgentSetter); ok {
		s.SetSubAgents(subAgents)
	}
}

// AddToolSet 转发 team coordinator 断言的接口（team/options.go）。
func (a *runScopedAgent) AddToolSet(ts trpctool.ToolSet) {
	if s, ok := a.inner.(interface{ AddToolSet(trpctool.ToolSet) }); ok {
		s.AddToolSet(ts)
	}
}

// RemoveToolSet / SetToolSets 转发 llmagent 在线热替换 API（P0-2B 将直接
// 作用于缓存 agent，包装器不得遮蔽）。
func (a *runScopedAgent) RemoveToolSet(name string) bool {
	if s, ok := a.inner.(interface{ RemoveToolSet(string) bool }); ok {
		return s.RemoveToolSet(name)
	}
	return false
}

func (a *runScopedAgent) SetToolSets(toolSets []trpctool.ToolSet) {
	if s, ok := a.inner.(interface{ SetToolSets([]trpctool.ToolSet) }); ok {
		s.SetToolSets(toolSets)
	}
}

// retireToolSets moves toolSets into the graveyard instead of closing them
// immediately: in-flight requests may still hold the replaced/evicted agent
// and be mid-call on its ToolSets (MCP sessions, stdio subprocesses); closing
// under their feet aborts those calls. The sweeper closes an entry as soon as
// its generational handle reports zero in-flight runs (P0-4), with retireDelay
// as the leak-detecting fallback (see sweepGraveyard). It also lazily starts
// the sweeper on first use. Caller must hold c.mu.
func (c *BuildCache) retireToolSets(toolSets []trpctool.ToolSet, cacheKey string, handle *agentHandle) {
	if len(toolSets) == 0 {
		return
	}
	if c.closed {
		// After Close the sweeper is gone; close immediately (async, caller
		// holds mu) so ToolSets are not leaked. Shutdown has no in-flight
		// requests left worth delaying for.
		safego.Go(context.Background(), "agent.cache.close_toolsets", func() { closeToolSetsNow(c.lg, toolSets, cacheKey) })
		return
	}
	c.graveyard = append(c.graveyard, retiredToolSet{
		toolSets: toolSets,
		cacheKey: cacheKey,
		retireAt: time.Now(),
		handle:   handle,
	})
	c.ensureSweeperLocked()
}

// ensureSweeperLocked starts the graveyard sweeper goroutine once. Caller
// must hold c.mu.
func (c *BuildCache) ensureSweeperLocked() {
	if c.sweeperCancel != nil {
		return
	}
	interval := toolSetSweepInterval
	if c.retireDelay > 0 && c.retireDelay < interval {
		interval = c.retireDelay
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.sweeperCancel = cancel
	c.sweeperDone = done
	lg := c.lg
	safego.Go(ctx, "agent.cache.toolset_sweeper", func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.sweepGraveyard(lg)
			}
		}
	})
}

// sweepGraveyard closes graveyard entries that are safe to reclaim (P0-4):
//   - refs==0（无在途 run 持有该代际）→ 本周期即关，不再白等 retireDelay；
//   - age≥retireDelay 且 refs>0 → 泄漏兜底：强制关闭 + Warn + 指标
//     （agent.cache.refcount_leak / aranea_agent_cache_refcount_leak_total），
//     把「run 未终结/引用未释放」从静默滞留变为可观测事件。
//
// Close calls run outside c.mu: ToolSet.Close may perform IO (MCP session
// teardown) and must not block cache operations.
func (c *BuildCache) sweepGraveyard(lg loggateway.Logger) {
	now := time.Now()
	c.mu.Lock()
	var due []retiredToolSet
	var leaked []retiredToolSet
	remaining := make([]retiredToolSet, 0, len(c.graveyard))
	for _, r := range c.graveyard {
		refs := r.handle.load()
		switch {
		case refs == 0:
			due = append(due, r)
		case now.Sub(r.retireAt) >= c.retireDelay:
			due = append(due, r)
			leaked = append(leaked, r)
		default:
			remaining = append(remaining, r)
		}
	}
	c.graveyard = remaining
	c.mu.Unlock()
	for _, r := range leaked {
		arametrics.AgentCacheRefcountLeaks.Inc()
		lg.Warn("退役 ToolSet 超兜底时长仍有在途引用，强制关闭（疑似 run 未终结或泄漏）",
			loggateway.StepID("agent.cache.refcount_leak"),
			loggateway.Str("cache_key", r.cacheKey),
			loggateway.Int64("refs", r.handle.load()),
			loggateway.Str("retired_for", now.Sub(r.retireAt).String()))
	}
	for _, r := range due {
		closeToolSetsNow(lg, r.toolSets, r.cacheKey)
	}
}

// closeToolSetsNow closes each ToolSet in the slice immediately, logging any
// errors. It is best-effort: a Close error on one ToolSet does not prevent
// the others from being closed.
func closeToolSetsNow(lg loggateway.Logger, toolSets []trpctool.ToolSet, cacheKey string) {
	for _, ts := range toolSets {
		if ts == nil {
			continue
		}
		if err := ts.Close(); err != nil {
			lg.Warn("ToolSet Close 失败",
				loggateway.Domain("agent.cache"),
				loggateway.Str("cache_key", cacheKey),
				loggateway.Str("toolset", ts.Name()),
				loggateway.Err(err))
		}
	}
}
