package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// --- P1 FR-6：车道等待移出会话锁临界区（M-1） ---

// TestRunNativeAgentTurn_LaneWaitIsCancellableAndOutsideSessionLock 复现 M-1：
// 旧顺序 lock → Sessions.Get → AcquireTurnLane 让被限流的 turn 持会话锁阻塞
// 于车道，同会话后续 turn 被连带拖死在互斥上，且取消无效（Lock 无 ctx 变体）。
//
// 编排：换入本测独占的干净 LaneScheduler（全局 DefaultLaneScheduler 可能被
// 套件内滞留 goroutine 占位——例：FR-4 resume goroutine 被 gate 钉在执行窗口，
// 其 durable 槽位永不释放；占满全局 4 槽的探测会把该泄漏放大为整包超时），
// 占满 durable 车道（limit=4）→ turn A（durable 入口）驻留车道等待 → turn B
// （同会话、durable 入口）携可取消 ctx 进入并取消。修复期望：B 的等待挂在
// 车道（honoring ctx），2s 内以 context.Canceled 返回；A 随 cleanup 取消同样
// 解除。缺陷行为：A 持会话锁阻塞在车道；B 阻塞在不可取消的 entry.mu 上，取消
// 无效 → 断言超时红（签名也未含 ok 返回）。
func TestRunNativeAgentTurn_LaneWaitIsCancellableAndOutsideSessionLock(t *testing.T) {
	// 调度器隔离（包内测试串行，无并发读写竞争；本测 turn 全部读取换入后的
	// 全局变量，cleanup 还原）。
	prevScheduler := rt.DefaultLaneScheduler
	rt.DefaultLaneScheduler = rt.NewLaneScheduler(rt.DefaultLaneLimits())
	t.Cleanup(func() { rt.DefaultLaneScheduler = prevScheduler })

	// 占满 durable 车道（本测独占调度器，limit=4）。
	releases := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		rel, ok := rt.DefaultLaneScheduler.Acquire(context.Background(), rt.TurnLaneDurable)
		if !ok {
			t.Fatal("precondition: durable lane slot must be acquirable")
		}
		releases = append(releases, rel)
	}
	t.Cleanup(func() {
		for _, rel := range releases {
			rel()
		}
	})

	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-lane-order"
	orch, _ := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	durableInput := func() biz.TurnInput {
		return biz.TurnInput{
			SessionID: sessionID,
			Content:   "durable-turn",
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint: biz.EntryPointDurable,
				AllowQueue: false,
			},
		}
	}

	// A：驻留车道等待（持锁与否即本测鉴别点）。
	turnCtxA, cancelA := context.WithCancel(context.Background())
	t.Cleanup(cancelA)
	doneA := make(chan struct{})
	go func() {
		defer close(doneA)
		_, _ = orch.RunNativeAgentTurnWithOutcome(turnCtxA, durableInput())
	}()
	time.Sleep(150 * time.Millisecond) // 让 A 越过会话加载、抵达车道等待

	// B：抵达等待后取消——修复期望经 ctx 解除。
	turnCtxB, cancelB := context.WithCancel(context.Background())
	doneB := make(chan error, 1)
	go func() {
		_, errB := orch.RunNativeAgentTurnWithOutcome(turnCtxB, durableInput())
		doneB <- errB
	}()
	time.Sleep(150 * time.Millisecond) // 让 B 抵达等待点
	cancelB()

	select {
	case errB := <-doneB:
		if !errors.Is(errB, context.Canceled) {
			t.Fatalf("P1 FR-6: cancelled waiter must surface context.Canceled, got %v", errB)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-6: wait outlived ctx cancel — lane wait still behind the session lock (M-1)")
	}

	cancelA()
	select {
	case <-doneA:
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-6: lane wait never released after ctx cancel")
	}
	reg.Finish(sessionID, "")
}
