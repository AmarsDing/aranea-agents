package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 钉板：澄清卡片挂起（turn 以阻断性提问收尾）后用户答复静默丢失缺陷
// （live44 2026-10-29 十轮复测 P1）。
//
// 缺陷链：挂起 turn 的 Finish 按 G2 设计把 session_runs 行保持 interactive
// （awaiting_user），但 runner 已随 turn 收尾关闭、框架队列不复存在；用户
// 答复（卡片作答或自由回复）走新 turn 续跑，Begin 撞 StartInteractive 并发
// 守卫后，G2 收养路径（adoptAwaitingRun）依赖的进程内 awaiting_user 标记
// 已被 RunRegistry.Finish 在挂起 turn  teardown 时清除（且续跑 turn 的
// early-ack SetRunStatus(running) 也会在 Begin 前覆盖它）→ 收养永不命中 →
// fail-closed CONFLICT → 答复消息从未落库（静默丢失）。
//
// 修复：澄清解除（卡片 submitClarification / 自由回复
// resolveClarificationFreeText）时把挂起的 interactive run 收尾为
// completed——问答回合已闭环，续跑 turn 开新 run。真并发冲突（无澄清
// 等待态）仍由 Begin fail-closed，不受本路径影响。

// heldRunRepoStub 持有单条挂起 session_run 的 repo stub：
// GetActiveForSession 仅在行 phase ∈ {interactive, durable} 且 FinishedAt
// 为空时命中（与 data.sessionRunRepo 同语义）；MarkTerminalWherePhase 走
// CAS（from 相位不符即失败），与生产 ent 实现一致。
type heldRunRepoStub struct {
	run       biz.SessionRun
	markCalls int
}

func (s *heldRunRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	if s.run.ID == id {
		return s.run, nil
	}
	return biz.SessionRun{}, apierror.NotFound(apierror.DomainSessionRun, "not found")
}

func (s *heldRunRepoStub) GetActiveForSession(_ context.Context, sessionID string) (biz.SessionRun, error) {
	if s.run.SessionID == sessionID && s.run.FinishedAt == "" &&
		(s.run.Phase == biz.SessionRunPhaseInteractive || s.run.Phase == biz.SessionRunPhaseDurable) {
		return s.run, nil
	}
	return biz.SessionRun{}, apierror.NotFound(apierror.DomainSessionRun, "not found")
}

func (s *heldRunRepoStub) ListBySession(context.Context, string, int, int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *heldRunRepoStub) ListForJobs(context.Context, biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *heldRunRepoStub) ListByPhase(context.Context, string, int) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *heldRunRepoStub) Create(context.Context, biz.SessionRun) (string, error) { return "", nil }
func (s *heldRunRepoStub) UpdateCheckpointID(context.Context, string, string) error {
	return nil
}
func (s *heldRunRepoStub) MarkTerminal(context.Context, string, string, string) error {
	return nil
}

func (s *heldRunRepoStub) MarkTerminalWherePhase(_ context.Context, id, fromPhase, toPhase, errMsg string) (bool, error) {
	s.markCalls++
	if s.run.ID != id || s.run.Phase != fromPhase {
		return false, nil
	}
	s.run.Phase = toPhase
	s.run.ErrorMessage = errMsg
	return true, nil
}

func (s *heldRunRepoStub) TryClaimDurableResume(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *heldRunRepoStub) ClearResumeClaim(context.Context, string) error { return nil }
func (s *heldRunRepoStub) MarkOrphanedRunsCancelled(context.Context) (int, error) {
	return 0, nil
}
func (s *heldRunRepoStub) TransitionPhase(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func newHeldRunTestLifecycle(repo biz.SessionRunRepo, reg *rt.RunRegistry) *chatSessionRunLifecycle {
	return newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns: biz.NewSessionRunUsecase(repo, nil, loggateway.NewNoop()),
		RunStatus:   noopRunStatusTracker{},
		Runs:        reg,
		Logger:      loggateway.NewNoop(),
	})
}

// wireHeldRunLifecycle 把澄清测试 orch 的 runMgr 换成携带真实
// chatSessionRunLifecycle（挂起 run repo stub）的版本，其余子管理器保持 noop。
func wireHeldRunLifecycle(orch *ChatOrchestrator, lc *chatSessionRunLifecycle) {
	orch.runMgr = &chatRunManagerImpl{
		runStatusTracker:    noopRunStatusTracker{},
		pendingQueueManager: noopPendingQueueManager{},
		awaitCoordinator:    noopAwaitCoordinator{},
		sessionRunLifecycle: lc,
	}
}

// 挂起的 interactive 行在 runner 已退场时必须被收尾为 completed。
func TestCompleteHeldSessionRun_CompletesParkedInteractiveRun(t *testing.T) {
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-hold", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	lc := newHeldRunTestLifecycle(repo, rt.NewRunRegistry())
	lc.CompleteHeldSessionRun(context.Background(), "sess-1")
	if repo.run.Phase != biz.SessionRunPhaseCompleted {
		t.Fatalf("held run phase = %s, want completed", repo.run.Phase)
	}
	if repo.markCalls != 1 {
		t.Fatalf("MarkTerminalWherePhase calls = %d, want 1", repo.markCalls)
	}
}

// runner 仍在跑（HITL-await 等在跑挂起）时禁止收尾——其行由在跑 turn 自身
// 的 Finish 负责。
func TestCompleteHeldSessionRun_SkipsWhileRunnerActive(t *testing.T) {
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-hold", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	reg := rt.NewRunRegistry()
	reg.StoreCancelable("sess-1", "run-live", func() {})
	lc := newHeldRunTestLifecycle(repo, reg)
	lc.CompleteHeldSessionRun(context.Background(), "sess-1")
	if repo.run.Phase != biz.SessionRunPhaseInteractive {
		t.Fatalf("active runner's row must not be finalized, got phase=%s", repo.run.Phase)
	}
	if repo.markCalls != 0 {
		t.Fatalf("MarkTerminalWherePhase calls = %d, want 0", repo.markCalls)
	}
}

// durable 行（升级待续跑）不归本路径收尾。
func TestCompleteHeldSessionRun_SkipsDurableRow(t *testing.T) {
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-durable", SessionID: "sess-1", Phase: biz.SessionRunPhaseDurable, CheckpointID: "cp-1",
	}}
	lc := newHeldRunTestLifecycle(repo, rt.NewRunRegistry())
	lc.CompleteHeldSessionRun(context.Background(), "sess-1")
	if repo.run.Phase != biz.SessionRunPhaseDurable {
		t.Fatalf("durable row must survive, got phase=%s", repo.run.Phase)
	}
	if repo.markCalls != 0 {
		t.Fatalf("MarkTerminalWherePhase calls = %d, want 0", repo.markCalls)
	}
}

// 无挂起行时静默 no-op（泄漏行之外的普通会话不受影响）。
func TestCompleteHeldSessionRun_NoActiveRowNoop(t *testing.T) {
	repo := &heldRunRepoStub{}
	lc := newHeldRunTestLifecycle(repo, rt.NewRunRegistry())
	lc.CompleteHeldSessionRun(context.Background(), "sess-1")
	if repo.markCalls != 0 {
		t.Fatalf("MarkTerminalWherePhase calls = %d, want 0", repo.markCalls)
	}
}

// 自由回复解除澄清时：既有语义（信封作答、输入改写、ParentTaskID 挂接）
// 不变，且挂起的 interactive run 必须收尾——否则续跑 turn 在 Begin 被
// fail-closed CONFLICT，答复静默丢失。
func TestResolveClarificationFreeText_FinalizesHeldRun(t *testing.T) {
	stepReader := &stubStepV2ReaderByID{steps: map[string]biz.Step{}}
	stepWriter := &recordingStepV2Writer{}
	orch := newClarifyFreeTextTestOrch(stepReader, stepWriter, &stubEventPublisher{}, &stubSessionStateTransitor{})
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-hold", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	wireHeldRunLifecycle(orch, newHeldRunTestLifecycle(repo, rt.NewRunRegistry()))

	step, pc := seedPendingClarifyStep(t, "sess-1")
	stepReader.steps[step.ID] = step
	orch.pendingClarifications.Store("sess-1", pc)

	got, _ := orch.resolveClarificationFreeText(context.Background(),
		biz.TurnInput{SessionID: "sess-1", Content: "做成 Web 端即可"})
	if got.ParentTaskID != pc.TaskID {
		t.Fatalf("resume input must attach ParentTaskID=%s, got %q", pc.TaskID, got.ParentTaskID)
	}
	if repo.run.Phase != biz.SessionRunPhaseCompleted {
		t.Fatalf("held run must be completed on clarify resolve, got phase=%s", repo.run.Phase)
	}
}

// 回归钉板：无澄清等待态时（如泄漏的 interactive 行），自由文本路径绝不
// 收尾 run——真并发/泄漏冲突仍由 Begin fail-closed 把关。
func TestResolveClarificationFreeText_NoPending_KeepsHeldRun(t *testing.T) {
	stepReader := &stubStepV2ReaderByID{steps: map[string]biz.Step{}}
	orch := newClarifyFreeTextTestOrch(stepReader, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-leak", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	wireHeldRunLifecycle(orch, newHeldRunTestLifecycle(repo, rt.NewRunRegistry()))

	got, _ := orch.resolveClarificationFreeText(context.Background(),
		biz.TurnInput{SessionID: "sess-1", Content: "新消息"})
	if got.Content != "新消息" {
		t.Fatalf("no pending clarification: input must pass through, got %q", got.Content)
	}
	if repo.run.Phase != biz.SessionRunPhaseInteractive {
		t.Fatalf("held run must stay interactive without clarify resolution, got phase=%s", repo.run.Phase)
	}
}

// 卡片作答解除澄清时同样收尾挂起 run（SubmitClarification RPC 路径）。
func TestSubmitClarification_FinalizesHeldRun(t *testing.T) {
	stepReader := &stubStepV2ReaderByID{steps: map[string]biz.Step{}}
	stepWriter := &recordingStepV2Writer{}
	orch := newClarifyFreeTextTestOrch(stepReader, stepWriter, &stubEventPublisher{}, &stubSessionStateTransitor{})
	repo := &heldRunRepoStub{run: biz.SessionRun{
		ID: "srid-hold", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	wireHeldRunLifecycle(orch, newHeldRunTestLifecycle(repo, rt.NewRunRegistry()))

	step, _ := seedPendingClarifyStep(t, "sess-1")
	stepReader.steps[step.ID] = step

	svc := &ChatService{orch: orch, lg: loggateway.NewNoop()}
	if _, err := svc.submitClarification(context.Background(), step, []biz.ClarificationAnswer{
		{Selected: []string{"Web"}},
	}); err != nil {
		t.Fatalf("submitClarification: %v", err)
	}
	if repo.run.Phase != biz.SessionRunPhaseCompleted {
		t.Fatalf("held run must be completed on card submit, got phase=%s", repo.run.Phase)
	}
}
