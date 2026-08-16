package agent

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	arametrics "aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// countingToolSet 统计 Close 调用次数（P1-5 泄漏不变量：退役数必须等于关闭数）。
type countingToolSet struct {
	name   string
	closed *atomic.Int64
}

func (f *countingToolSet) Tools(context.Context) []trpctool.Tool { return nil }
func (f *countingToolSet) Close() error                          { f.closed.Add(1); return nil }
func (f *countingToolSet) Name() string                          { return f.name }

// TestBuildCacheRetiredToolSetDelayedClose verifies the graveyard contract:
// ToolSets retired via eviction or replacement are NOT closed immediately
// (in-flight requests may still be mid-call on them); the sweeper closes
// them on its next tick once no in-flight run references the retired
// generation (P0-4 引用计数语义，refs==0 即关；retireDelay 退化为泄漏兜底）。
// Close() closes them at once.
func TestBuildCacheRetiredToolSetDelayedClose(t *testing.T) {
	c := newTestCache(1)
	c.retireDelay = 20 * time.Millisecond

	ts := &fakeToolSet{name: "ts-old"}
	// Evict via LRU overflow (cap=1).
	c.put("a", makeAgent("a"), nil, []trpctool.ToolSet{ts})
	c.put("b", makeAgent("b"), nil, nil)

	if ts.closed.Load() {
		t.Fatal("retired ToolSet must not be closed immediately after eviction")
	}
	c.mu.Lock()
	inGraveyard := len(c.graveyard)
	c.mu.Unlock()
	if inGraveyard != 1 {
		t.Fatalf("expected 1 graveyard entry after eviction, got %d", inGraveyard)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !ts.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !ts.closed.Load() {
		t.Fatal("sweeper must close the retired ToolSet after the delay elapses")
	}
	c.mu.Lock()
	left := len(c.graveyard)
	cancel := c.sweeperCancel
	c.mu.Unlock()
	if left != 0 {
		t.Fatalf("expected empty graveyard after sweep, got %d entries", left)
	}
	if cancel == nil {
		t.Fatal("sweeper should have been started by retireToolSets")
	}
	c.Close()
}

// TestBuildCache_ChurnLeakInvariants（P1-5，Cordis E2/E6 元理论的测试化）：
// 「构建→失效→重建」循环 N=50 次后断言泄漏不变量回基线：
//   - 每轮 sweep 后 graveyard 归零（退役 ToolSet 被精确回收，无滞留）；
//   - 全部创建的 ToolSet 最终都被 Close（MCP session/stdio 子进程的单测
//     等价物——真实 ToolSet 的 Close 即释放其 MCP 会话）；
//   - active refs 指标归零（无悬挂在途引用）；
//   - goroutine 数回基线 ±2（pump/sweeper 无泄漏；包级 goleak 另有兜底）。
//
// 人为注入泄漏（注释掉 sweepGraveyard 的 closeToolSetsNow，或 pump 的
// release）时本测试必须变红——这是后续所有波次的回归网。
func TestBuildCache_ChurnLeakInvariants(t *testing.T) {
	runtime.GC()
	goroutineBase := runtime.NumGoroutine()

	c := newTestCache(4)
	c.retireDelay = time.Hour // 兜底不参与：全程只验证精确回收路径（refs==0 即关）

	const N = 50
	var closedCount atomic.Int64
	created := 0

	// runOneGeneration 模拟一次「构建（put）→ 在途 run → run 终结」。
	runOneGeneration := func(key string) {
		runCh := make(chan *trpcevent.Event)
		ts := &countingToolSet{name: fmt.Sprintf("ts-%d", created), closed: &closedCount}
		created++
		h := c.put(key, &controllableAgent{runCh: runCh}, nil, []trpctool.ToolSet{ts})
		if h == nil {
			t.Fatal("put must return generation handle")
		}
		evCh, err := h.agent.Run(context.Background(), trpcagent.NewInvocation())
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		close(runCh)
		for range evCh { // 排空 → pump 释放引用（release 先于 out 关闭）
		}
	}
	assertGraveyardDrained := func(round string) {
		t.Helper()
		c.sweepGraveyard(c.lg)
		c.mu.Lock()
		left := len(c.graveyard)
		c.mu.Unlock()
		if left != 0 {
			t.Fatalf("%s: graveyard = %d entries after sweep, want 0 (retired ToolSet leaked)", round, left)
		}
	}

	// 路径一：同 key 反复替换（覆盖「替换退役」路径，即配置变更重建形态）。
	for i := 0; i < N; i++ {
		runOneGeneration("hot")
		assertGraveyardDrained(fmt.Sprintf("replace round %d", i))
	}
	// 路径二：多 key 轮换撑爆容量（覆盖「LRU 驱逐退役」路径）。
	for i := 0; i < N; i++ {
		runOneGeneration(fmt.Sprintf("cold-%d", i))
		assertGraveyardDrained(fmt.Sprintf("evict round %d", i))
	}

	// 在途引用指标必须归零（pump 全部退出）。
	if got := testutil.ToFloat64(arametrics.AgentCacheActiveRefs); got != 0 {
		t.Fatalf("AgentCacheActiveRefs = %v after churn, want 0 (dangling in-flight ref)", got)
	}

	// Close 排水中缓存里残存的最后几代；此后所有创建过的 ToolSet 必须全部
	// 被 Close——任何一个未关闭即 ToolSet 泄漏（对应生产即 MCP 会话泄漏）。
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, want := closedCount.Load(), int64(created); got != want {
		t.Fatalf("closed ToolSets = %d, want %d (created); %d leaked", got, want, want-got)
	}

	// goroutine 回基线 ±2（sweeper 已停、pump 全退；容许测试进程后台计时器抖动）。
	if got := runtime.NumGoroutine(); got > goroutineBase+2 {
		t.Fatalf("goroutines = %d after churn+Close, base = %d (tolerance +2)", got, goroutineBase)
	}
}
