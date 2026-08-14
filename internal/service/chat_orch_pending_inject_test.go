package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/loggateway"
)

// ─── P2-3 Inbox 三级注入语义：pending-queue 派发循环集成测试 ─────────────────
//
// 覆盖 chat_orchestrator_turn_dispatch.go 的 inject 合入行为：
//   - 头部连续 inject 条目不单独唤醒 turn，与第一条 followup 合并为一次派发；
//   - 仅剩 inject 时保持排队、零派发；
//   - 合并后的内容经 DLQ（turn 失败路径）可完整观测。
//
// team 路径被选用作派发通道：TeamsNative 是接口可直接桩掉快速失败，
// 避免触及单 agent 路径的 buildTurnRunner 真实构建。

// captureTeamRunner 记录每次派发的 TurnInput 并立即失败（触发 DLQ 捕获原文）。
type captureTeamRunner struct {
	mu     sync.Mutex
	inputs []biz.TurnInput
}

func (r *captureTeamRunner) RunTurnFromInput(_ context.Context, _ biz.Session, input biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	r.mu.Lock()
	r.inputs = append(r.inputs, input)
	r.mu.Unlock()
	return biz.ChatMessage{}, biz.ChatMessage{}, errors.New("team run failed (test stub)")
}

func (r *captureTeamRunner) SetMediator(biz.TeamMediatorPort)                 {}
func (r *captureTeamRunner) SetAwaitHookProvider(biz.AwaitHookProvider)       {}
func (r *captureTeamRunner) SetDeliverableGate(biz.TeamDeliverableGateFunc)   {}
func (r *captureTeamRunner) SetQualityGate(biz.TeamQualityGateFunc)           {}
func (r *captureTeamRunner) SetRevisionEnqueuer(biz.TeamRevisionEnqueuerFunc) {}
func (r *captureTeamRunner) SetUpstreamDeliverableSeed(biz.TeamUpstreamSeedFunc) {}

func (r *captureTeamRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.inputs)
}

func (r *captureTeamRunner) first() biz.TurnInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.inputs) == 0 {
		return biz.TurnInput{}
	}
	return r.inputs[0]
}

func newPendingInjectOrch(t *testing.T, q *rt.PendingMessageQueue, dlq *lifecycle.DeadLetterQueue, runner biz.TeamRunnerWirePort) *ChatOrchestrator {
	t.Helper()
	lockMgr := biz.NewSessionLockManager()
	t.Cleanup(lockMgr.Close)
	return &ChatOrchestrator{
		runs:   rt.NewRunRegistry(),
		chatUC: biz.NewChatUsecase(nil, lockMgr, NewPendingQueueAdapter(q), nil, nil, loggateway.NewNoop()),
		turnLC: newNoopChatTurnLifecycle(),
		runMgr: newNoopChatRunManager(),
		teamExecDeps: ChatTeamDeps{Team: TeamOrchestrationDeps{
			TeamsNative: runner,
		}},
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop(), DeadLetterQueue: dlq},
	}
}

func waitForCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestProcessPendingQueue_InjectsMergeIntoFollowup(t *testing.T) {
	q := rt.NewPendingMessageQueue()
	dlq := lifecycle.NewDeadLetterQueue(8, loggateway.NewNoop())
	runner := &captureTeamRunner{}
	orch := newPendingInjectOrch(t, q, dlq, runner)

	const sid = "sess-inject-merge"
	q.EnqueueInject(sid, "上下文A：用户偏好中文回复")
	q.EnqueueInject(sid, "上下文B：项目代号 Aranea")
	q.Enqueue(sid, "总结一下当前状态") // 无 kind = followup

	sess := biz.Session{ID: sid, AgentID: "agent-1", OwnerType: "team"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1"}
	orch.processPendingQueue(sid, sess, ag, "", "", "", "")

	// team runner 必失败 → DLQ 落信，以此作为派发完成的确定性信号。
	waitForCond(t, "dlq entry", func() bool { return dlq.Len() > 0 })
	// 等循环走到第二轮（队列空 → 返回），避免队列断言竞态。
	waitForCond(t, "queue drain", func() bool { return len(q.List(sid)) == 0 })

	if got := runner.count(); got != 1 {
		t.Fatalf("dispatched turns = %d, want 1 (injects must merge into followup, not wake separately)", got)
	}
	want := biz.MergeInjectContext([]string{"上下文A：用户偏好中文回复", "上下文B：项目代号 Aranea"}, "总结一下当前状态")
	if got := runner.first().Content; got != want {
		t.Fatalf("merged content mismatch:\n got %q\nwant %q", got, want)
	}
	msgs := dlq.List(1)
	if len(msgs) != 1 || msgs[0].Original != want {
		t.Fatalf("dlq = %+v, want 1 entry carrying merged content", msgs)
	}
	if !strings.HasPrefix(want, "[系统上下文补充]") {
		t.Fatal("merged content must carry inject context header")
	}
}

func TestProcessPendingQueue_OnlyInjectsStaySilent(t *testing.T) {
	q := rt.NewPendingMessageQueue()
	dlq := lifecycle.NewDeadLetterQueue(8, loggateway.NewNoop())
	runner := &captureTeamRunner{}
	orch := newPendingInjectOrch(t, q, dlq, runner)

	const sid = "sess-inject-only"
	q.EnqueueInject(sid, "系统上下文：定时任务已注册")
	q.EnqueueInject(sid, "系统上下文：监控规则已更新")

	sess := biz.Session{ID: sid, AgentID: "agent-1", OwnerType: "team"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1"}
	orch.processPendingQueue(sid, sess, ag, "", "", "", "")

	// inject 不唤醒 turn：宽限期内必须零派发、零 DLQ、队列原样保留。
	time.Sleep(300 * time.Millisecond)
	if got := runner.count(); got != 0 {
		t.Fatalf("dispatched turns = %d, want 0 (inject-only queue must stay silent)", got)
	}
	if dlq.Len() != 0 {
		t.Fatalf("dlq len = %d, want 0", dlq.Len())
	}
	remaining := q.List(sid)
	if len(remaining) != 2 || remaining[0].Kind != biz.ChatEnqueueKindInject || remaining[1].Kind != biz.ChatEnqueueKindInject {
		t.Fatalf("remaining = %+v, want 2 inject entries preserved in order", remaining)
	}
}
