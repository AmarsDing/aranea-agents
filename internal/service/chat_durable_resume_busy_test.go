package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- P0 FR-4：durable resume busy 瞬态化 + claim 占位 ---

// newDurableResumeTestService 构造 ResumeDurableSessionRun 最小闭环：
// durableWorkerRepoStub/checkpoint stub（SessionRun 仓储）+ 生产同款 turn
// 接线（newPendingRegTestOrch：真实 RunRegistry/SessionLockManager）。
func newDurableResumeTestService(t *testing.T, reg *rt.RunRegistry, gate *gatedRunStatusMgr, sessionID, sessionRunID string) (*ChatService, *durableWorkerRepoStub) {
	t.Helper()
	bus := &captureEventBus{}
	orch, _ := newPendingRegTestOrch(t, reg, bus, gate, sessionID)
	repo := &durableWorkerRepoStub{runs: map[string]biz.SessionRun{
		sessionRunID: {
			ID:           sessionRunID,
			SessionID:    sessionID,
			Phase:        biz.SessionRunPhaseDurable,
			CheckpointID: "cp-1",
		},
	}}
	cps := &durableWorkerCheckpointStub{cps: map[string]biz.SessionRunCheckpoint{
		"cp-1": {ID: "cp-1", SessionRunID: sessionRunID, PayloadJSON: `{"session_id":"` + sessionID + `","turn_id":"t1","agent_id":"agent-1"}`},
	}}
	uc := biz.NewSessionRunUsecase(repo, cps, loggateway.NewNoop())
	orch.channelDeps = ChatChannelDeps{ChJobs: ChannelTurnJobDeps{SessionRuns: uc}}
	return &ChatService{lg: loggateway.NewNoop(), orch: orch}, repo
}

// hookCheckpointStub 让 GetBySessionRunID（ResumeDurableSessionRun 的读取
// 入口）附带副作用钩子——用于确定性构造 "claim 成功 → checkpoint 读取" 与
// "resume turn 注册" 之间的并发窗口。
type hookCheckpointStub struct {
	durableWorkerCheckpointStub
	onGet func()
}

func (s *hookCheckpointStub) GetBySessionRunID(ctx context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	if s.onGet != nil {
		s.onGet()
	}
	return s.durableWorkerCheckpointStub.GetBySessionRunID(ctx, sessionRunID)
}

// TestResumeDurableSessionRun_BusyTransientClearsClaim 复现 H-2：claim 成功
// 后、resume goroutine 启动并到达准入前，同会话被并发 turn 抢占槽位——
// resume turn 必得 BUSY。这是瞬态冲突而非失败。修复期望：清 claim 交还
// （不 Fail、不通知、不转状态），worker 下一 tick 重试。
// 缺陷行为：busy 按失败处理 Fail(run) → durable run 永久死亡。
func TestResumeDurableSessionRun_BusyTransientClearsClaim(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{chatRunManager: newNoopChatRunManager(), entered: make(chan struct{}), release: make(chan struct{})}
	const sessionID = "sess-fr4-busy"
	orch, _ := newPendingRegTestOrch(t, reg, bus, gate, sessionID)
	repo := &durableWorkerRepoStub{runs: map[string]biz.SessionRun{
		"run-fr4": {ID: "run-fr4", SessionID: sessionID, Phase: biz.SessionRunPhaseDurable, CheckpointID: "cp-1"},
	}}
	// 确定性时序：:34 HasActiveRun 检查通过（槽位空）→ claim → GetCheckpoint
	// 期间并发 turn 抢占槽位 → goroutine 的 resume turn 锁内 TOCTOU 必判 busy。
	cps := &hookCheckpointStub{onGet: func() {
		reg.StoreCancelable(sessionID, "other-active-run", func() {})
	}}
	cps.cps = map[string]biz.SessionRunCheckpoint{
		"cp-1": {ID: "cp-1", SessionRunID: "run-fr4", PayloadJSON: `{"session_id":"` + sessionID + `","turn_id":"t1","agent_id":"agent-1"}`},
	}
	uc := biz.NewSessionRunUsecase(repo, cps, loggateway.NewNoop())
	orch.channelDeps = ChatChannelDeps{ChJobs: ChannelTurnJobDeps{SessionRuns: uc}}
	chat := &ChatService{lg: loggateway.NewNoop(), orch: orch}

	if err := chat.ResumeDurableSessionRun(context.Background(), "run-fr4"); err != nil {
		t.Fatal(err)
	}

	var run biz.SessionRun
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run = repo.get("run-fr4")
		// 收敛：claim 被清除（瞬态分支 ClearResumeClaim 或旧代码 Fail 内的
		// 清理）即 goroutine 已走完 busy 路径，可下定论。
		if run.ResumeStartedAt == "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("diag: claimed=%q phase=%q err=%q hasActive=%v placeholderOnly=%v gateCalls=%d",
		run.ResumeStartedAt, run.Phase, run.ErrorMessage, reg.HasActive(sessionID), reg.IsPlaceholderOnly(sessionID), gate.calls.Load())
	if run.ResumeStartedAt != "" {
		t.Fatalf("P0 FR-4: busy resume must clear resume claim (still claimed=%q)", run.ResumeStartedAt)
	}
	if run.Phase != biz.SessionRunPhaseDurable {
		t.Fatalf("P0 FR-4: busy is transient, run must stay durable (not failed), got phase=%q err=%q", run.Phase, run.ErrorMessage)
	}
	if run.ErrorMessage != "" {
		t.Fatalf("P0 FR-4: transient busy must not record failure, got err=%q", run.ErrorMessage)
	}
	reg.Finish(sessionID, "")
}

// TestResumeDurableSessionRun_PlaceholderAdoptedByDurableTurn 验证占位采用：
// claim 成功后登记的占位（claim→turn 注册窗口对外 busy 的护栏）必须由
// 携 DurableResume 标记的本 resume turn 采用，而不是把自己 busy 掉。
//
// 编排：正常 resume（槽位空闲）→ 轮到 resume turn 锁内 TOCTOU。
// 修复期望：占位被采用并升级为真实条目（SetRunStatus 到达、Phase 仍 durable、
// 槽位不再是"未触碰占位"）。
// 缺陷行为（改造前）：占位会挡住 resume turn 自己 → 永远走 busy 清 claim 循环。
func TestResumeDurableSessionRun_PlaceholderAdoptedByDurableTurn(t *testing.T) {
	reg := rt.NewRunRegistry()
	gate := &gatedRunStatusMgr{chatRunManager: newNoopChatRunManager(), entered: make(chan struct{}), release: make(chan struct{})}
	const sessionID = "sess-fr4-adopt"
	chat, repo := newDurableResumeTestService(t, reg, gate, sessionID, "run-fr4-adopt")

	if err := chat.ResumeDurableSessionRun(context.Background(), "run-fr4-adopt"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("P0 FR-4: resume turn blocked by its own claim placeholder (never reached SetRunStatus)")
	}
	// 占位已被采用并升级（cancel 落到槽位）→ 不再是"未触碰占位"。
	if reg.IsPlaceholderOnly(sessionID) {
		t.Fatal("P0 FR-4: placeholder must be adopted (upgraded) by the resume turn, not left bare")
	}
	if run := repo.get("run-fr4-adopt"); run.Phase != biz.SessionRunPhaseDurable {
		t.Fatalf("P0 FR-4: adopted resume turn must not have failed the run, got phase=%q", run.Phase)
	}
	reg.Finish(sessionID, "")
}

// TestPlaceholderRejectsNonDurableTurn 对照组：占位槽位对非 durable 标记的
// turn 必须呈现 busy，且占位本身不被破坏——这正是 claim 窗口护栏的职责。
func TestPlaceholderRejectsNonDurableTurn(t *testing.T) {
	reg := rt.NewRunRegistry()
	gate := &gatedRunStatusMgr{chatRunManager: newNoopChatRunManager(), entered: make(chan struct{}), release: make(chan struct{})}
	const sessionID = "sess-fr4-guard"
	orch, _ := func() (*ChatOrchestrator, *biz.ChatUsecase) {
		bus := &captureEventBus{}
		return newPendingRegTestOrch(t, reg, bus, gate, sessionID)
	}()
	reg.StorePlaceholder(sessionID)

	_, err := orch.RunNativeAgentTurnWithOutcome(context.Background(), biz.TurnInput{
		SessionID: sessionID,
		Content:   "hello",
	})
	if !isTurnBusyError(err) {
		t.Fatalf("P0 FR-4: non-durable turn over claim placeholder must be rejected busy, got err=%v", err)
	}
	if !reg.IsPlaceholderOnly(sessionID) {
		t.Fatal("P0 FR-4: rejected turn must not disturb the claim placeholder")
	}
	reg.Finish(sessionID, "")
}
