package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- P1 FR-7：重启恢复队列惰性排空（新消息先强制入队，再整体 FIFO 排空）---

// restoredStoreStub 预置持久层恢复行，供 PendingMessageQueue.SetStore 加载。
type restoredStoreStub struct {
	rows map[string][]rt.PendingMessage
}

func (s *restoredStoreStub) LoadAll(context.Context) (map[string][]rt.PendingMessage, error) {
	return s.rows, nil
}

func (s *restoredStoreStub) ReplaceAll(context.Context, map[string][]rt.PendingMessage) error {
	return nil
}

func restoredRow(id, content string) rt.PendingMessage {
	return rt.PendingMessage{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}

// newRestoredFifoTestOrch 复刻 newPendingRegTestOrch 的最小闭环，额外回传
// pending 句柄供 SetStore 预置"重启恢复行"。
func newRestoredFifoTestOrch(t *testing.T, reg *rt.RunRegistry, bus *captureEventBus, gate *gatedRunStatusMgr, sessionID string) (*ChatOrchestrator, *biz.ChatUsecase, *rt.PendingMessageQueue) {
	t.Helper()
	pending := rt.NewPendingMessageQueue()
	t.Cleanup(pending.Close)
	lockMgr := biz.NewSessionLockManager()
	t.Cleanup(lockMgr.Close)
	chatUC := biz.NewChatUsecase(
		NewRunGatewayAdapter(reg),
		lockMgr,
		NewPendingQueueAdapter(pending),
		nil,
		NewChatEventPublisher(nil, nil),
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
	return orch, chatUC, pending
}

func pendingContents(pending *rt.PendingMessageQueue, sessionID string) []string {
	list := pending.List(sessionID)
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.Content)
	}
	return out
}

func equalStrings(a, b []string) bool {
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

// TestRunNativeAgentTurn_RestoredQueueForcesEnqueueAndDrainsFIFO 复现 FR-7
// 根因（重启后 FIFO 反转）：会话空闲 + 持久层恢复行 [F1,F2] 存在时，新消息
// 直行会插队到 F1 之前。修复期望：准入前强制入队 [F1,F2,NEW] → 触发整体
// FIFO 排空（F1 先行出队执行）→ 返回 queued。
func TestRunNativeAgentTurn_RestoredQueueForcesEnqueueAndDrainsFIFO(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}), // 驻留窗口：F1 钉在执行段，测末进程回收
	}
	const sessionID = "sess-restored-fifo"
	orch, _, pending := newRestoredFifoTestOrch(t, reg, bus, gate, sessionID)
	pending.SetStore(&restoredStoreStub{rows: map[string][]rt.PendingMessage{
		sessionID: {restoredRow("f1", "F1"), restoredRow("f2", "F2")},
	}})

	result, err := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
		SessionID: sessionID,
		Content:   "NEW",
	})
	if !errors.Is(err, ErrTurnMessageQueued) && !isTurnMessageQueued(err) {
		t.Fatalf("P1 FR-7: idle session with restored rows must queue the new message, got err=%v", err)
	}
	if result.Outcome != biz.TurnOutcomeQueued || result.PendingID == "" {
		t.Fatalf("P1 FR-7: outcome must be queued with pending id, got %+v", result)
	}

	// FIFO：NEW 不得插队。
	if got := pendingContents(pending, sessionID); !equalStrings(got, []string{"F1", "F2", "NEW"}) {
		t.Fatalf("P1 FR-7: FIFO violated, queue=%v", got)
	}

	// 惰性排空已被触发：队首 F1 出队进入执行窗口。
	select {
	case <-gate.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-7: restored queue was not drained lazily after enqueue")
	}
	if got := pendingContents(pending, sessionID); !equalStrings(got, []string{"F2", "NEW"}) {
		t.Fatalf("P1 FR-7: restored head must be dequeued first (FIFO), queue=%v", got)
	}
	reg.Finish(sessionID, "")
}

// TestRunNativeAgentTurn_NoRestoredRowsRunsDirectly 对照组：会话无恢复行时
// 准入前 hook 必须静默——新消息照常直行执行，不产生排队。
func TestRunNativeAgentTurn_NoRestoredRowsRunsDirectly(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-fresh-direct"
	orch, _, pending := newRestoredFifoTestOrch(t, reg, bus, gate, sessionID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
			SessionID: sessionID,
			Content:   "direct",
		})
		// 执行窗口被 gate 钉住：调用不会返回（gate 解锁于进程退出）；仅在
		// 意外返回时校验它不是"被排队"。
		if isTurnMessageQueued(err) || result.Outcome == biz.TurnOutcomeQueued {
			t.Errorf("P1 FR-7: fresh session must not be queued by the restored hook")
		}
	}()

	select {
	case <-gate.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-7: fresh-session turn must proceed directly to execution")
	}
	if got := pendingContents(pending, sessionID); len(got) != 0 {
		t.Fatalf("P1 FR-7: fresh-session turn must not create queue entries, got %v", got)
	}
	reg.Finish(sessionID, "")
}

// TestRunNativeAgentTurn_RestoredQueueSkippedWhenQueueNotAllowed 对照组：
// 不允许排队的入口（durable 等 plan-driven 通道）不触发强制入队——恢复
// FIFO 语义仅服务于可排队会话面。
func TestRunNativeAgentTurn_RestoredQueueSkippedWhenQueueNotAllowed(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-restored-noqueue"
	orch, _, pending := newRestoredFifoTestOrch(t, reg, bus, gate, sessionID)
	pending.SetStore(&restoredStoreStub{rows: map[string][]rt.PendingMessage{
		sessionID: {restoredRow("f1", "F1"), restoredRow("f2", "F2")},
	}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
			SessionID: sessionID,
			Content:   "durable-turn",
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint: biz.EntryPointDurable,
				AllowQueue: false,
			},
		})
		if isTurnMessageQueued(err) || result.Outcome == biz.TurnOutcomeQueued {
			t.Errorf("P1 FR-7: AllowQueue=false entry must bypass the restored hook")
		}
	}()

	select {
	case <-gate.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("P1 FR-7: AllowQueue=false turn must proceed directly to execution")
	}
	// 恢复行原样保留，新消息未入队。
	if got := pendingContents(pending, sessionID); !equalStrings(got, []string{"F1", "F2"}) {
		t.Fatalf("P1 FR-7: queue must be untouched for no-queue entries, got %v", got)
	}
	reg.Finish(sessionID, "")
}
