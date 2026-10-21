package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- P0 FR-1：pending-queue 出队 turn 必须"注册先于放锁" ---

// gatedRunStatusMgr 包装 noop run manager：首个 SetRunStatus 发出 entered
// 信号后驻留（release 永不关闭）——把出队 turn 确定性钉在"已出队、已（应）
// 注册、EXECUTE 前段"的执行窗口内，供并发 turn 观察注册表。
type gatedRunStatusMgr struct {
	chatRunManager
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (g *gatedRunStatusMgr) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error {
	g.calls.Add(1)
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.chatRunManager.SetRunStatus(ctx, sessionID, runID, status, errMsg)
}

// newPendingRegTestOrch 构造出队路径最小闭环：真实 RunRegistry + 真实
// SessionLockManager + 真实 PendingMessageQueue（生产同款 adapter 接线）+
// noop 会话/agent 桩。gated runMgr 把 A 钉在执行窗口。
func newPendingRegTestOrch(t *testing.T, reg *rt.RunRegistry, bus *captureEventBus, gate *gatedRunStatusMgr, sessionID string) (*ChatOrchestrator, *biz.ChatUsecase) {
	t.Helper()
	pending := rt.NewPendingMessageQueue()
	lockMgr := biz.NewSessionLockManager()
	t.Cleanup(lockMgr.Close)
	chatUC := biz.NewChatUsecase(
		NewRunGatewayAdapter(reg),
		lockMgr,
		NewPendingQueueAdapter(pending),
		nil,                            // persister：测试路径未触
		NewChatEventPublisher(nil, nil), // seq/bus 双 nil → 入队通知 no-op
		loggateway.NewNoop(),
	)
	sessStub := &toctouSessionManager{
		sess: biz.Session{ID: sessionID, AgentID: "agent-1", OwnerType: "agent"},
	}
	evtPub := newChatTurnEventPublisher(nil, bus, nil, loggateway.NewNoop())
	orch := &ChatOrchestrator{
		runs:   reg,
		chatUC: chatUC,
		core: chatTurnCoreDeps{
			TD: rt.TurnDeps{
				Sessions: sessStub,
				ReadDeps: rt.TurnReadDeps{
					Agents: &toctouAgentRepo{ag: biz.Agent{
						ID: "agent-1", AgentKey: "agent-1", Provider: "p", Model: "m",
					}},
				},
			},
		},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
		runMgr:    gate,
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}
	return orch, chatUC
}

// TestProcessPendingQueue_RegisterBeforeUnlock_ConcurrentTurnRejected 复现
// C-1：出队 turn 在 unlock 前未注册，执行窗口内 HasActive=false，准入层放
// 行并发 turn → 双跑。
//
// 时序（确定性编排）：
//  1. 队列预置一条 followup；启动 processPendingQueue。
//  2. A 出队、（修复期望）unlock 前 StoreCancelable(regID)、unlock、进入
//     runSingleAgentViaTRPC，被 gated SetRunStatus 钉在执行窗口。
//  3. 断言窗口内注册表 HasActive=true——这是全部后续保护的根。
//  4. 并发 turn B（AgentKey 不匹配，若被放行会在 admitTurn 干净失败）：
//     修复期望 B 被准入拒绝（BUSY/入队），绝不产生 TaskFailedEvent。
//
// 缺陷行为（修复前）：窗口内 HasActive=false，B 一路放行到 admitTurn 失败 →
// bus 出现 TaskFailedEvent（双跑实锤），errB 为 FORBIDDEN 而非 busy/queued。
func TestProcessPendingQueue_RegisterBeforeUnlock_ConcurrentTurnRejected(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}), // 永不放行：A 驻留窗口，测试进程退出时回收
	}
	const sessionID = "sess-pending-reg"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	chatUC.EnqueuePendingMessage(sessionID, "queued-followup")

	sess := biz.Session{ID: sessionID, AgentID: "agent-1", OwnerType: "agent"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1", Provider: "p", Model: "m"}
	orch.processPendingQueue(sessionID, sess, ag, "default", "p", "m", "")

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("pending turn did not reach SetRunStatus — dequeue/admission chain broken")
	}

	// 核心不变量：出队 turn 执行窗口内注册表必须可见（注册先于放锁）。
	if !reg.HasActive(sessionID) {
		t.Fatal("P0 FR-1: dequeued pending turn must be registered before unlock — HasActive=false in execution window")
	}

	_, errB := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
		SessionID: sessionID,
		Content:   "concurrent-message",
		AgentKey:  "other-agent", // 若被放行：admitTurn 干净失败并产 TaskFailedEvent
	})
	if !(isTurnBusyError(errB) || errors.Is(errB, ErrTurnMessageQueued)) {
		t.Fatalf("P0 FR-1: concurrent turn must be rejected busy/queued while pending dequeued turn is in flight, got err=%v", errB)
	}

	// B 绝不到达执行：无任何 TaskFailedEvent。
	failedCount := 0
	for _, ev := range bus.snapshot() {
		if _, ok := ev.(*biz.TaskFailedEvent); ok {
			failedCount++
		}
	}
	if failedCount != 0 {
		t.Fatalf("P0 FR-1: concurrent turn reached execution (TaskFailedEvent=%d) — double run", failedCount)
	}
	reg.Finish(sessionID, "")
}

// TestProcessPendingQueue_SingleFlight 复现 H-4：排空循环无 single-flight
//（chat_orchestrator.go 的 inPendingLoop(appctx.Ctx()) 重入守卫恒假），重复
// 触发会起并发排空循环。
//
// 编排：A 被钉在执行窗口（loop-1 存活）；再次触发 processPendingQueue：
//   - 修复期望：第二次触发被合并（同步 LoadOrStore），SetRunStatus,calls 恒 1，
//     队列中第二条消息不被并发出队。
//   - 缺陷行为：loop-2 并行启动，出队 F2 并第二次调 SetRunStatus。
func TestProcessPendingQueue_SingleFlight(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-pending-sf"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	chatUC.EnqueuePendingMessage(sessionID, "followup-1")
	chatUC.EnqueuePendingMessage(sessionID, "followup-2")

	sess := biz.Session{ID: sessionID, AgentID: "agent-1", OwnerType: "agent"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1", Provider: "p", Model: "m"}
	orch.processPendingQueue(sessionID, sess, ag, "default", "p", "m", "")

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop-1 pending turn did not reach SetRunStatus")
	}

	// loop-1 持有会话消息 F1 驻留执行窗口；此刻再次触发排空。
	orch.processPendingQueue(sessionID, sess, ag, "default", "p", "m", "")

	// 观察一个稳定窗：若 loop-2 并行跑起来，会 dequeue F2 并第二次
	// SetRunStatus（calls=2，队列清空）。稳定窗内 calls 恒 1 即合并生效。
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if gate.calls.Load() != 1 {
			t.Fatalf("P0 FR-1: second pending loop ran concurrently (SetRunStatus calls=%d) — single-flight missing", gate.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 1 {
		t.Fatalf("P0 FR-1: expected 1 message still queued (F2 untouched by merged loop), got %d", n)
	}
	reg.Finish(sessionID, "")
}

// --- P0 FR-2/FR-3 ---

// panickingTransitor 在 runSingleAgentViaTRPC 的状态转换步（unlock 之后）
// 注入 panic，复现 FR-3"StoreCancelable 与 EXECUTE defer 之间"的泄漏窗口。
type panickingTransitor struct{ noopSessionStateTransitor }

func (panickingTransitor) TransitionStatus(context.Context, string, sessstatus.SessionStatus, sessstatus.SessionStatusReason) {
	panic("boom: injected panic after registration")
}

// TestRunNativeAgentTurn_EarlyFailureDrainsPendingQueue 复现 H-1：正常入口
// turn 早退失败（admitTurn FORBIDDEN）后，排队消息不得搁浅——错误分支必须
// 触发排空循环。
//
// 编排：
//  1. 队列预置 F1；A（AgentKey 不匹配）走正常入口 admit 失败。
//  2. 修复期望：错误分支排空 → F1 出队注册 → gated SetRunStatus 命中、队列清空。
//  3. 缺陷行为：无人触发排空 —— gate 信号超时，消息永久搁浅。
func TestRunNativeAgentTurn_EarlyFailureDrainsPendingQueue(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-pending-drain"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	chatUC.EnqueuePendingMessage(sessionID, "queued-followup")

	_, errA := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
		SessionID: sessionID,
		Content:   "hello",
		AgentKey:  "other-agent", // admitTurn 干净失败（FORBIDDEN），走早退分支
	})
	if errA == nil {
		t.Fatal("expected admit failure for mismatched agent key")
	}

	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("P0 FR-2: early-failure path never drains the pending queue — message stranded")
	}
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 0 {
		t.Fatalf("P0 FR-2: expected queue drained after early failure, %d message(s) remain", n)
	}
	reg.Finish(sessionID, "")
}

// TestRunNativeAgentTurn_PanicDoesNotLeakRegistry 复现 M-2：StoreCancelable
// 之后、EXECUTE defer 之前的 panic 会残留注册项 → 会话永久 busy。
//
// 编排：A 通过 admit（AgentKey 匹配），SetRunStatus 后于状态转换步 panic。
// 修复期望：注册点 defer Finish（CAS 幂等）兜底，panic 后注册表无残留。
// 缺陷行为：panic 穿透错误分支（err 路径不触发），agentRunID 永远残留。
func TestRunNativeAgentTurn_PanicDoesNotLeakRegistry(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{chatRunManager: newNoopChatRunManager(), entered: make(chan struct{}), release: make(chan struct{})}
	close(gate.release) // SetRunStatus 直通：panic 点在其后
	const sessionID = "sess-panic-leak"
	orch, _ := newPendingRegTestOrch(t, reg, bus, gate, sessionID)
	orch.turnLC = &chatTurnLifecycleImpl{
		sessionStateTransitor: panickingTransitor{},
		turnRecorder:          noopTurnRecorder{},
		turnEventPublisher:    newChatTurnEventPublisher(nil, bus, nil, loggateway.NewNoop()),
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
			SessionID: sessionID,
			Content:   "hello",
			AgentKey:  "agent-1", // admit 通过 → SetRunStatus → transitionSessionStatus panic
		})
	}()
	if recovered == nil {
		t.Fatal("expected injected panic to propagate")
	}
	if reg.HasActive(sessionID) {
		t.Fatal("P0 FR-3: registry entry leaked after panic — session permanently busy")
	}
}
