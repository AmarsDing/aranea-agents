package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	arametrics "aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// controllableAgent 的 Run 返回测试可控的事件通道：不关闭即视为「在途 run」。
type controllableAgent struct {
	mockAgent
	runCh  chan *trpcevent.Event
	runErr error
}

func (a *controllableAgent) Run(context.Context, *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	if a.runErr != nil {
		return nil, a.runErr
	}
	return a.runCh, nil
}

// TestRunScopedAgent_InFlightRunBlocksRetireClose（AC-1）：在途 run 期间替换
// 代际，旧 ToolSet 不得被 sweeper 关闭；run 事件流终结后下一个清扫周期关闭。
func TestRunScopedAgent_InFlightRunBlocksRetireClose(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = time.Hour // 兜底不参与：全程只看 refs 语义

	old := &fakeToolSet{name: "ts-old"}
	runCh := make(chan *trpcevent.Event)
	h1 := c.put("k", &controllableAgent{runCh: runCh}, nil, []trpctool.ToolSet{old})
	if h1 == nil {
		t.Fatal("put must return generation handle")
	}

	// 在旧代际上开启在途 run。
	evCh, err := h1.agent.Run(context.Background(), trpcagent.NewInvocation())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := h1.load(); got != 1 {
		t.Fatalf("refs after Run start = %d, want 1", got)
	}

	// 替换代际 → 旧 ToolSet 携旧句柄退役；refs=1，sweeper 不得关闭。
	c.put("k", makeAgent("v2"), nil, nil)
	c.sweepGraveyard(c.lg)
	if old.closed.Load() {
		t.Fatal("retired ToolSet must stay open while a run is in flight")
	}

	// run 终结（事件通道关闭）→ 引用归零 → 下一清扫周期关闭。
	close(runCh)
	for range evCh { // 排空，等待 pump 释放
	}
	if got := h1.load(); got != 0 {
		t.Fatalf("refs after run end = %d, want 0", got)
	}
	c.sweepGraveyard(c.lg)
	if !old.closed.Load() {
		t.Fatal("retired ToolSet must close on the sweep after refs drain to 0")
	}
	c.Close()
}

// TestRunScopedAgent_IdleReplaceClosesNextSweep（AC-2）：无在途 run 的替换，
// 引用归零即关，不等 retireDelay。
func TestRunScopedAgent_IdleReplaceClosesNextSweep(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = time.Hour // 若错误地走兜底语义，本测试将永不关闭而失败

	old := &fakeToolSet{name: "ts-old"}
	c.put("k", makeAgent("v1"), nil, []trpctool.ToolSet{old})
	c.put("k", makeAgent("v2"), nil, nil) // 无任何 Run，refs==0

	c.sweepGraveyard(c.lg)
	if !old.closed.Load() {
		t.Fatal("idle retired ToolSet must close on the first sweep (refs==0), not wait retireDelay")
	}
	c.Close()
}

// TestRunScopedAgent_LeakFallbackForceCloses（AC-3）：人为构造引用滞留
// （run 永不终结），超 retireDelay 后兜底强制关闭并累计泄漏指标。
func TestRunScopedAgent_LeakFallbackForceCloses(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = 20 * time.Millisecond

	old := &fakeToolSet{name: "ts-old"}
	runCh := make(chan *trpcevent.Event) // 断言前不关闭：模拟 run 未终结
	h1 := c.put("k", &controllableAgent{runCh: runCh}, nil, []trpctool.ToolSet{old})
	evCh, err := h1.agent.Run(context.Background(), trpcagent.NewInvocation())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c.put("k", makeAgent("v2"), nil, nil) // 退役时 refs=1

	before := testutil.ToFloat64(arametrics.AgentCacheRefcountLeaks)
	time.Sleep(30 * time.Millisecond) // 超过 retireDelay
	c.sweepGraveyard(c.lg)

	if !old.closed.Load() {
		t.Fatal("leak fallback must force-close retired ToolSet past retireDelay")
	}
	after := testutil.ToFloat64(arametrics.AgentCacheRefcountLeaks)
	if after-before != 1 {
		t.Fatalf("refcount leak metric delta = %v, want 1", after-before)
	}
	// 兜底关闭后 graveyard 必须清空（不留重复告警）。
	c.mu.Lock()
	left := len(c.graveyard)
	c.mu.Unlock()
	if left != 0 {
		t.Fatalf("graveyard = %d entries after fallback close, want 0", left)
	}
	// 收尾：终结滞留的 run，让 pump 协程退出（goleak）。
	close(runCh)
	for range evCh {
	}
	c.Close()
}

// TestRunScopedAgent_RunErrorReleasesRef：Run 立即失败时引用当场归还。
func TestRunScopedAgent_RunErrorReleasesRef(t *testing.T) {
	c := newTestCache(4)
	h := c.put("k", &controllableAgent{runErr: errors.New("boom")}, nil, nil)
	if _, err := h.agent.Run(context.Background(), trpcagent.NewInvocation()); err == nil {
		t.Fatal("expected run error")
	}
	if got := h.load(); got != 0 {
		t.Fatalf("refs after failed Run = %d, want 0", got)
	}
	c.Close()
}

// TestRunScopedAgent_ConsumerAbandonReleasesRef：消费者放弃事件流（ctx 取消）
// 时 pump 退出并释放引用。
func TestRunScopedAgent_ConsumerAbandonReleasesRef(t *testing.T) {
	c := newTestCache(4)
	runCh := make(chan *trpcevent.Event, 1)
	h := c.put("k", &controllableAgent{runCh: runCh}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	inv := trpcagent.NewInvocation()
	evCh, err := h.agent.Run(ctx, inv)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	runCh <- &trpcevent.Event{} // 让 pump 进入下一次 EmitEvent
	<-evCh
	cancel()
	runCh <- &trpcevent.Event{} // 触发 EmitEvent 失败路径（ctx 已取消）

	deadline := time.Now().Add(2 * time.Second)
	for h.load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.load(); got != 0 {
		t.Fatalf("refs after consumer abandon = %d, want 0", got)
	}
	c.Close()
}

// TestRunScopedAgent_ConcurrentChurnRace（AC-4）：并发「构建替换 + 在途 run +
// 释放」压力，验证计数与 graveyard 无竞态（-race 下运行）。
func TestRunScopedAgent_ConcurrentChurnRace(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = 50 * time.Millisecond

	const workers = 8
	const rounds = 25
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				runCh := make(chan *trpcevent.Event)
				ts := &fakeToolSet{name: "ts"}
				h := c.put("k", &controllableAgent{runCh: runCh}, nil, []trpctool.ToolSet{ts})
				if h == nil {
					continue
				}
				evCh, err := h.agent.Run(context.Background(), trpcagent.NewInvocation())
				if err == nil {
					close(runCh) // 立即终结 run
					for range evCh {
					}
				}
			}
		}(w)
	}
	wg.Wait()
	c.sweepGraveyard(c.lg)
	// 全部 run 已终结：graveyard 中所有条目 refs==0，一次清扫后必须为空。
	c.mu.Lock()
	left := len(c.graveyard)
	c.mu.Unlock()
	if left != 0 {
		t.Fatalf("graveyard = %d entries after final sweep, want 0", left)
	}
	c.Close()
}

// TestRunScopedAgent_ForwardsCapabilityInterfaces：team 框架类型断言依赖的
// 可选接口必须透传到内层 agent。
func TestRunScopedAgent_ForwardsCapabilityInterfaces(t *testing.T) {
	inner := &capabilityAgent{}
	h := newAgentHandle(inner)
	wrapped := h.agent

	setter, ok := wrapped.(trpcagent.SubAgentSetter)
	if !ok {
		t.Fatal("wrapper must satisfy agent.SubAgentSetter (team swarm assertion)")
	}
	setter.SetSubAgents([]trpcagent.Agent{makeAgent("sub")})
	if inner.subAgentsSet != 1 {
		t.Fatalf("SetSubAgents forwarded %d times, want 1", inner.subAgentsSet)
	}

	adder, ok := wrapped.(interface{ AddToolSet(trpctool.ToolSet) })
	if !ok {
		t.Fatal("wrapper must satisfy AddToolSet (team coordinator assertion)")
	}
	adder.AddToolSet(&fakeToolSet{name: "x"})
	if inner.toolSetsAdded != 1 {
		t.Fatalf("AddToolSet forwarded %d times, want 1", inner.toolSetsAdded)
	}

	remover, ok := wrapped.(interface{ RemoveToolSet(string) bool })
	if !ok {
		t.Fatal("wrapper must satisfy RemoveToolSet (P0-2B hot-replace path)")
	}
	if !remover.RemoveToolSet("x") || inner.toolSetsRemoved != 1 {
		t.Fatal("RemoveToolSet must forward and return inner result")
	}
}

// capabilityAgent 记录可选接口调用次数。
type capabilityAgent struct {
	mockAgent
	subAgentsSet   int
	toolSetsAdded  int
	toolSetsRemoved int
}

func (a *capabilityAgent) SetSubAgents([]trpcagent.Agent) { a.subAgentsSet++ }
func (a *capabilityAgent) AddToolSet(trpctool.ToolSet)    { a.toolSetsAdded++ }
func (a *capabilityAgent) RemoveToolSet(string) bool      { a.toolSetsRemoved++; return true }
