package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/loggateway"
)

// ── D1-F1：pending-queue 团队分支取消分流 ──────────────────────────────────
// 87-orchestration-chain-hardening：出队的团队 turn 被用户停止
// （context.Canceled）时必须走 cancel 终态——根 Task cancelled、会话
// interrupted(user_cancelled)、HandleTeamTurnResult(cancelled, errMsg 原文)——
// 并跳过 publishTurnFailure 与 DLQ Enqueue（取消不是失败邮件，重投会把用户
// 明确停止的任务再次执行）。

// pendingRecordStarter 是带互斥的 biz.TeamStarterPort 桩（processPendingQueue
// 在后台 goroutine 派发，断言侧需要同步观测点）。
type pendingRecordStarter struct {
	mu     sync.Mutex
	calls  int
	status string
	errMsg string
}

func (s *pendingRecordStarter) StartTeamTurn(context.Context, string, string) error { return nil }

func (s *pendingRecordStarter) HandleTeamTurnResult(_ context.Context, _, _, status, errMsg, _ string) {
	s.mu.Lock()
	s.calls++
	s.status = status
	s.errMsg = errMsg
	s.mu.Unlock()
}

func (s *pendingRecordStarter) snapshot() (int, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.status, s.errMsg
}

// pendingRecordTransitor 记录 TransitionStatus(target, reason) 序列。
type pendingRecordTransitor struct {
	mu     sync.Mutex
	status []sessstatus.SessionStatus
	reason []sessstatus.SessionStatusReason
}

func (s *pendingRecordTransitor) TransitionStatus(_ context.Context, _ string, target sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	s.mu.Lock()
	s.status = append(s.status, target)
	s.reason = append(s.reason, reason)
	s.mu.Unlock()
}

func (s *pendingRecordTransitor) has(target sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.status {
		if s.status[i] == target && s.reason[i] == reason {
			return true
		}
	}
	return false
}

// pendingRecordPublisher 记录 PublishTurnFailure 次数（取消路径必须为零）。
type pendingRecordPublisher struct {
	mu         sync.Mutex
	failCalls  int
	lastSource string
}

func (s *pendingRecordPublisher) PublishTurnFailure(_, _, source string, _ error, _ string) {
	s.mu.Lock()
	s.failCalls++
	s.lastSource = source
	s.mu.Unlock()
}

func (s *pendingRecordPublisher) BumpSessionRevision(context.Context, string) {}

func (s *pendingRecordPublisher) failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failCalls
}

// newPendingCancelOrch 组合 newTeamTurnTestOrch（团队 runner/starter/任务库）
// 与 newPendingInjectOrch（真实 pending queue + DLQ）的最小依赖集。
func newPendingCancelOrch(t *testing.T, q *rt.PendingMessageQueue, dlq *lifecycle.DeadLetterQueue,
	runner biz.TeamRunnerWirePort, starter biz.TeamStarterPort, taskRepo biz.TaskV2Repo,
	lc chatTurnLifecycle) *ChatOrchestrator {
	t.Helper()
	lockMgr := biz.NewSessionLockManager()
	t.Cleanup(lockMgr.Close)
	return &ChatOrchestrator{
		core:   chatTurnCoreDeps{TaskV2: taskRepo},
		runs:   rt.NewRunRegistry(),
		chatUC: biz.NewChatUsecase(nil, lockMgr, NewPendingQueueAdapter(q), nil, nil, loggateway.NewNoop()),
		teamExecDeps: ChatTeamDeps{
			Team:        TeamOrchestrationDeps{TeamsNative: runner},
			TeamStarter: starter,
		},
		turnLC:    lc,
		runMgr:    newNoopChatRunManager(),
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop(), DeadLetterQueue: dlq},
	}
}

// TestProcessPendingQueue_TeamCancelled_SkipsFailureAndDLQ pins：出队团队
// turn 的 runner 返回 context.Canceled → 全量取消终态落账，且 publishTurnFailure
// 与 DLQ 都被跳过。
func TestProcessPendingQueue_TeamCancelled_SkipsFailureAndDLQ(t *testing.T) {
	q := rt.NewPendingMessageQueue()
	dlq := lifecycle.NewDeadLetterQueue(8, loggateway.NewNoop())
	runner := &capturingTeamRunner{runErr: context.Canceled}
	starter := &pendingRecordStarter{}
	writer := &stubTaskV2Writer{}
	transitor := &pendingRecordTransitor{}
	publisher := &pendingRecordPublisher{}
	lc := &chatTurnLifecycleImpl{
		sessionStateTransitor: transitor,
		turnRecorder:          noopTurnRecorder{},
		turnEventPublisher:    publisher,
	}
	orch := newPendingCancelOrch(t, q, dlq, runner, starter, &stubTaskV2Repo{writer: writer}, lc)

	const sid = "team-sess-pending-cancel"
	q.Enqueue(sid, "排队中的团队任务")
	sess := biz.Session{ID: sid, OwnerType: "team", TeamID: "team-1", ParentSessionID: "spirit-1"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1"}

	orch.processPendingQueue(sid, sess, ag, "", "", "", "")
	// 派发在后台 goroutine；starter 回调是分支内最后一次写，作为完成信号。
	waitForCond(t, "cancelled HandleTeamTurnResult", func() bool {
		calls, _, _ := starter.snapshot()
		return calls == 1
	})

	if len(writer.terminalized) != 1 {
		t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(writer.terminalized))
	}
	if got := writer.terminalized[0].Status; got != biz.TaskStatusCancelled {
		t.Errorf("pending root Task Status = %q, want %q (user cancel is not failure)", got, biz.TaskStatusCancelled)
	}
	if !transitor.has(sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonUserCancelled) {
		t.Errorf("missing transition interrupted(user_cancelled), got status=%v reason=%v", transitor.status, transitor.reason)
	}
	calls, status, errMsg := starter.snapshot()
	if calls != 1 || status != biz.TeamStatusCancelled {
		t.Fatalf("HandleTeamTurnResult calls=%d status=%q, want 1/%q", calls, status, biz.TeamStatusCancelled)
	}
	if errMsg != context.Canceled.Error() {
		t.Errorf("errMsg = %q, want raw %q (取消原文保留，供下游分类器)", errMsg, context.Canceled.Error())
	}
	if got := publisher.failures(); got != 0 {
		t.Errorf("PublishTurnFailure calls = %d, want 0 (取消不是失败)", got)
	}
	if got := dlq.Len(); got != 0 {
		t.Errorf("dlq len = %d, want 0 (取消不重投用户明确停止的任务)", got)
	}
}

// TestProcessPendingQueue_TeamGenericErr_StillFailsAndDLQs pins 回归边界：
// 非取消错误保持既有 failed 路径（根 Task failed + interrupted(error) +
// 回调 failed + publishTurnFailure + DLQ 落信）。
func TestProcessPendingQueue_TeamGenericErr_StillFailsAndDLQs(t *testing.T) {
	q := rt.NewPendingMessageQueue()
	dlq := lifecycle.NewDeadLetterQueue(8, loggateway.NewNoop())
	runner := &capturingTeamRunner{runErr: errors.New("llm provider down")}
	starter := &pendingRecordStarter{}
	writer := &stubTaskV2Writer{}
	transitor := &pendingRecordTransitor{}
	publisher := &pendingRecordPublisher{}
	lc := &chatTurnLifecycleImpl{
		sessionStateTransitor: transitor,
		turnRecorder:          noopTurnRecorder{},
		turnEventPublisher:    publisher,
	}
	orch := newPendingCancelOrch(t, q, dlq, runner, starter, &stubTaskV2Repo{writer: writer}, lc)

	const sid = "team-sess-pending-fail"
	q.Enqueue(sid, "排队中的团队任务")
	sess := biz.Session{ID: sid, OwnerType: "team", TeamID: "team-1", ParentSessionID: "spirit-1"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1"}

	orch.processPendingQueue(sid, sess, ag, "", "", "", "")
	waitForCond(t, "dlq entry", func() bool { return dlq.Len() > 0 })

	if len(writer.terminalized) != 1 {
		t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(writer.terminalized))
	}
	if got := writer.terminalized[0].Status; got != biz.TaskStatusFailed {
		t.Errorf("pending root Task Status = %q, want %q (generic error stays failed)", got, biz.TaskStatusFailed)
	}
	if !transitor.has(sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError) {
		t.Errorf("missing transition interrupted(error), got status=%v reason=%v", transitor.status, transitor.reason)
	}
	calls, status, _ := starter.snapshot()
	if calls != 1 || status != biz.TeamStatusFailed {
		t.Fatalf("HandleTeamTurnResult calls=%d status=%q, want 1/%q", calls, status, biz.TeamStatusFailed)
	}
	if got := publisher.failures(); got != 1 {
		t.Errorf("PublishTurnFailure calls = %d, want 1", got)
	}
	if got := dlq.Len(); got != 1 {
		t.Errorf("dlq len = %d, want 1", got)
	}
}
