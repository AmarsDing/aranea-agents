package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- P1 TOCTOU 竞态复现桩件 ---

// toctouSessionManager 嵌入复合接口，仅覆写 Get：首个调用者（持锁的 turn A）
// 在临界区内发出信号并阻塞，模拟生产中的 DB 往返窗口（Sessions.Get →
// hydratedAgent → quota 之间的数十毫秒）。
type toctouSessionManager struct {
	biz.SessionTurnManager
	sess     biz.Session
	onFirst  func()
	getCalls atomic.Int32
}

func (s *toctouSessionManager) Get(_ context.Context, _ string) (biz.Session, error) {
	if s.getCalls.Add(1) == 1 && s.onFirst != nil {
		s.onFirst()
	}
	return s.sess, nil
}

// toctouAgentRepo 嵌入复合接口，仅覆写 GetAgentByID。
type toctouAgentRepo struct {
	biz.AgentRepository
	ag biz.Agent
}

func (r *toctouAgentRepo) GetAgentByID(_ context.Context, _ string) (biz.Agent, error) {
	return r.ag, nil
}

// toctouLocker 包装真实 SessionLockManager，在第二个 Lock 调用（turn B 到达
// 临界区入口、即将因 A 持锁而阻塞）时发出信号，使竞态时序确定性复现。
type toctouLocker struct {
	delegate *biz.SessionLockManager
	calls    atomic.Int32
	onSecond func()
}

func (l *toctouLocker) Lock(sessionID string) func() {
	if l.calls.Add(1) == 2 && l.onSecond != nil {
		l.onSecond()
	}
	return l.delegate.Lock(sessionID)
}

// gatedFailureBus 在首个 TaskFailedEvent 发布处阻塞，直至测试放行。
// 钉死「A：admitTurn 失败 → publishTurnFailure → runs.Finish」的执行顺序，
// 使 A 的注册项在 B 的锁内 HasActive 复查期间确定性在册——否则 A 释放会话锁
// 后与 B 并发竞速（unlock→Finish 仅数微秒），B 复查时 A 可能已 Finish，
// HasActive=false 导致 B 被放行至 admitTurn 收到 FORBIDDEN 而非 BUSY
//（2026-08-29 全量运行 flake 根因；生产语义上彼时 B 放行本属正确，
// 是测试把「复查时 A 在册」误当确定性前提）。超时兜底防回归挂死。
type gatedFailureBus struct {
	*captureEventBus
	release <-chan struct{}
	once    sync.Once
}

func (b *gatedFailureBus) Publish(ctx context.Context, e biz.Event) {
	if _, ok := e.(*biz.TaskFailedEvent); ok {
		b.once.Do(func() {
			select {
			case <-b.release:
			case <-time.After(10 * time.Second):
			}
		})
	}
	b.captureEventBus.Publish(ctx, e)
}

// TestRunNativeAgentTurn_TOCTOU_SecondTurnMustNotRun 复现 P1：两个并发 turn
// 对同一会话的"双跑"竞态。
//
// 时序（确定性编排）：
//  1. turn A 通过锁外 HasActive 快速检查（false），获取会话锁，阻塞在
//     Sessions.Get（模拟 DB 窗口）——此时尚未 StoreCancelable。
//  2. turn B 通过锁外 HasActive 快速检查（false，A 未存储），阻塞在会话锁上。
//  3. A 放行：StoreCancelable(A) → 释放锁 → admitTurn 因 AgentKey 不匹配
//     失败，首个 TaskFailedEvent 发布被 gatedFailureBus 挂起（A 的注册项
//     因此钉在册，不会先 Finish）。
//  4. B 获得锁：锁内 HasActive 复查命中 A → 拒绝为 CHAT_TURN_BUSY。
//  5. 测试放行事件闸门：A 完成 publishTurnFailure → Finish → 退出。
//
// 期望行为（修复后）：B 在锁内复查发现活跃运行，被拒为 CHAT_TURN_BUSY，
// 仅 A 到达 admitTurn（恰好 1 个 TaskFailedEvent）。
// 当前缺陷行为：B 也到达 admitTurn（2 个 TaskFailedEvent，双跑实锤），
// 且 B 的 StoreCancelable 覆盖 A 的 cancel func，A 成为不可取消的孤儿运行。
func TestRunNativeAgentTurn_TOCTOU_SecondTurnMustNotRun(t *testing.T) {
	reg := rt.NewRunRegistry()
	lockMgr := biz.NewSessionLockManager()
	t.Cleanup(lockMgr.Close)

	aInCritical := make(chan struct{})
	releaseA := make(chan struct{})
	bWaiting := make(chan struct{})

	sessStub := &toctouSessionManager{
		sess: biz.Session{ID: "sess-toctou", AgentID: "agent-1", OwnerType: "agent"},
		onFirst: func() {
			close(aInCritical)
			<-releaseA
		},
	}
	locker := &toctouLocker{delegate: lockMgr, onSecond: func() { close(bWaiting) }}
	chatUC := biz.NewChatUsecase(nil, locker, nil, nil, nil, loggateway.NewNoop())

	// 失败事件闸门：A 在 admitTurn 失败（publishTurnFailure）处挂起，直到 B
	// 完成锁内复查——保证 B 复查时 A 的注册项确定性在册（见 gatedFailureBus）。
	gate := make(chan struct{})
	bus := &gatedFailureBus{captureEventBus: &captureEventBus{}, release: gate}
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
		runMgr:    newNoopChatRunManager(),
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}

	// AgentKey 故意不匹配：让获胜 turn 在 admitTurn 处干净失败（publishTurnFailure
	// + 返回错误），避免触及 buildTurnRunner 的真实构建路径。
	input := biz.TurnInput{SessionID: "sess-toctou", Content: "hello", AgentKey: "other-agent"}

	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errA = orch.RunNativeAgentTurnWithOutcome(context.Background(), input)
	}()
	<-aInCritical // A 持锁并阻塞于 Sessions.Get（尚未 StoreCancelable）
	bDone := make(chan struct{})
	go func() {
		defer wg.Done()
		defer close(bDone)
		_, errB = orch.RunNativeAgentTurnWithOutcome(context.Background(), input)
	}()
	<-bWaiting      // B 已通过锁外 HasActive 快速检查，正阻塞在会话锁上
	close(releaseA) // A 放行：StoreCancelable → 释放锁 → admitTurn 失败（挂起在事件闸门）
	<-bDone         // B 完成锁内复查（A 的注册项被闸门钉在册）→ BUSY 被拒
	close(gate)     // 放行 A 的失败事件发布 → Finish → 退出
	wg.Wait()

	// 获胜 turn A 必然在 admitTurn 失败（AgentKey 不匹配）。
	if errA == nil {
		t.Fatal("expected winning turn A to fail at admitTurn (agent key mismatch)")
	}

	// 期望（修复后）：B 被拒为 CHAT_TURN_BUSY，绝不进入执行。
	if !isTurnBusyError(errB) {
		t.Fatalf("P1 TOCTOU: second concurrent turn must be rejected with CHAT_TURN_BUSY, got err=%v", errB)
	}

	// 期望（修复后）：仅 A 到达 admitTurn → 恰好 1 个 TaskFailedEvent。
	failedCount := 0
	for _, ev := range bus.snapshot() {
		if _, ok := ev.(*biz.TaskFailedEvent); ok {
			failedCount++
		}
	}
	if failedCount != 1 {
		t.Fatalf("P1 TOCTOU: expected exactly 1 TaskFailedEvent (winner only), got %d — double run", failedCount)
	}

	// 兜底清理：A 的早退路径已 Finish（chat_orchestrator_turn.go L249），
	// 此行防未来路径变动导致的残留。
	reg.Finish("sess-toctou", "")
}
