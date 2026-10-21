package runtime

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestLaneScheduler_limitsCron(t *testing.T) {
	sched := NewLaneScheduler(LaneLimits{Cron: 1})
	release, ok := sched.Acquire(context.Background(), TurnLaneCron)
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if sched.InFlight(TurnLaneCron) != 1 {
		t.Fatalf("inflight=%d", sched.InFlight(TurnLaneCron))
	}
	blockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	release2, ok2 := sched.Acquire(blockCtx, TurnLaneCron)
	if ok2 {
		release2()
		t.Fatal("P1 FR-6: acquire on full lane with expiring ctx must report ok=false")
	}
	release()
	if sched.InFlight(TurnLaneCron) != 0 {
		t.Fatal("slot should be released")
	}
	release3, ok3 := sched.Acquire(context.Background(), TurnLaneCron)
	if !ok3 {
		t.Fatal("third acquire must succeed")
	}
	if sched.InFlight(TurnLaneCron) != 1 {
		t.Fatal("third acquire should succeed")
	}
	release3()
}

// TestLaneScheduler_CancelWhileQueued 复现 M-1(lane 侧)：取消路径返回 no-op
// release 且不可区分——调用方无法得知槽位未取得，会放行无槽位 turn。
// 修复期望：Acquire 返回 (release, ok)；ctx 取消 → ok=false 且槽位数守恒。
func TestLaneScheduler_CancelWhileQueued(t *testing.T) {
	sched := NewLaneScheduler(LaneLimits{Cron: 1})
	release, ok := sched.Acquire(context.Background(), TurnLaneCron)
	if !ok {
		t.Fatal("precondition: first acquire must succeed")
	}
	t.Cleanup(release)

	waitCtx, cancelWait := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		_, ok := sched.Acquire(waitCtx, TurnLaneCron)
		done <- ok
	}()
	time.Sleep(50 * time.Millisecond) // 让 waiter 入队
	cancelWait()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("P1 FR-6: cancelled wait must return ok=false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-6: cancelled wait never returned (ctx not honored)")
	}

	// 守恒：holder 释放后槽位可被后续正常 Acquire 获得。
	release()
	rel2, ok2 := sched.Acquire(context.Background(), TurnLaneCron)
	if !ok2 {
		t.Fatal("slot leaked: acquire after cancelled waiter must succeed")
	}
	rel2()
	if sched.InFlight(TurnLaneCron) != 0 {
		t.Fatalf("inflight drift after churn: %d", sched.InFlight(TurnLaneCron))
	}
}

// TestLaneScheduler_CancelSignalHandoff 丢失唤醒守卫（行为级）：取消公平的
// waiter 后释放槽位，其后位 waiter 必须获得——取消不得吞掉槽位信号。
// 信号前置送达而 cancel 胜出 select 时的"转交义务"契约由
// TestLaneScheduler_DropWaiterHandoff 白盒固定。
func TestLaneScheduler_CancelSignalHandoff(t *testing.T) {
	sched := NewLaneScheduler(LaneLimits{Cron: 1})
	holder, ok := sched.Acquire(context.Background(), TurnLaneCron)
	if !ok {
		t.Fatal("precondition: first acquire must succeed")
	}

	ctxA, cancelA := context.WithCancel(context.Background())
	doneA := make(chan bool, 1)
	go func() {
		_, ok := sched.Acquire(ctxA, TurnLaneCron)
		doneA <- ok
	}()
	time.Sleep(50 * time.Millisecond) // A 入队

	doneB := make(chan bool, 1)
	go func() {
		rel, ok := sched.Acquire(context.Background(), TurnLaneCron)
		if ok {
			rel()
		}
		doneB <- ok
	}()
	time.Sleep(50 * time.Millisecond) // B 入队于 A 之后

	// 先取消 A（此时无其它就绪分支，ctx 分支确定），再释放 holder——无论
	// release 与 A 的 dropWaiter 谁先谁后，B 都必须获得槽位。
	cancelA()
	select {
	case okA := <-doneA:
		if okA {
			t.Fatal("cancelled waiter A must return ok=false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter A never returned after cancel")
	}
	holder()
	select {
	case okB := <-doneB:
		if !okB {
			t.Fatal("P1 FR-6: waiter B starved — cancelled waiter A swallowed the slot signal")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-6: waiter B never acquired — lost wakeup after cancelled waiter")
	}
}

// TestLaneScheduler_DropWaiterHandoff 白盒固定 dropWaiter 契约：
// 信号未送达 → false；已送达（release send 已发生但 waiter 走取消分支）
// → true（调用方须代其转交唤醒）。
func TestLaneScheduler_DropWaiterHandoff(t *testing.T) {
	sched := NewLaneScheduler(LaneLimits{Cron: 1})

	// 未投递信号：无转交义务。
	w1 := &laneWaiter{ch: make(chan struct{}, 1)}
	sched.mu.Lock()
	sched.wait[TurnLaneCron] = []*laneWaiter{w1}
	sched.mu.Unlock()
	if sched.dropWaiter(TurnLaneCron, w1) {
		t.Fatal("unsignalled waiter must report no handoff obligation")
	}
	if len(sched.wait[TurnLaneCron]) != 0 {
		t.Fatal("waiter must be removed from queue")
	}

	// 已投递信号（缓冲已填充）：有转交义务。
	w2 := &laneWaiter{ch: make(chan struct{}, 1)}
	w2.ch <- struct{}{}
	if !sched.dropWaiter(TurnLaneCron, w2) {
		t.Fatal("pre-delivered signal must report handoff obligation")
	}

	// dropped 标记对发行方可见：signalNext 不得再向 dropped waiter 投递。
	w3 := &laneWaiter{ch: make(chan struct{}, 1)}
	sched.mu.Lock()
	sched.wait[TurnLaneCron] = []*laneWaiter{w3}
	sched.mu.Unlock()
	sched.dropWaiter(TurnLaneCron, w3) // 无信号
	sched.signalNext(TurnLaneCron)
	select {
	case <-w3.ch:
		t.Fatal("signalNext must skip dropped waiters")
	default:
	}
}

func TestTurnLaneFromEntry(t *testing.T) {
	if TurnLaneFromEntry(biz.EntryPointCron, "agent") != TurnLaneCron {
		t.Fatal("cron lane")
	}
	if TurnLaneFromEntry(biz.EntryPointDurable, "agent") != TurnLaneDurable {
		t.Fatal("durable lane")
	}
	if TurnLaneFromEntry(biz.EntryPointChannel, "team") != TurnLaneTeam {
		t.Fatal("team owner")
	}
	if TurnLaneFromEntry(biz.EntryPointWS, "agent") != TurnLaneMain {
		t.Fatal("main lane")
	}
}

func TestLaneScheduler_mainUnlimited(t *testing.T) {
	sched := NewLaneScheduler(DefaultLaneLimits())
	for i := 0; i < 8; i++ {
		rel, ok := sched.Acquire(context.Background(), TurnLaneMain)
		if !ok {
			t.Fatal("main lane unlimited: acquire must always succeed")
		}
		rel()
	}
	if sched.InFlight(TurnLaneMain) != 0 {
		t.Fatalf("main lane should be unlimited, inflight=%d", sched.InFlight(TurnLaneMain))
	}
}
