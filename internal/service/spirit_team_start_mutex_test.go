package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// ── D2-②：StartTeamTurn 运行注册 + 会话锁 + 中止语义 ────────────────────────
// 87-orchestration-chain-hardening：dispatch/调度入口与 chat 入口共享同一
// RunRegistry + 会话互斥，消除同一 team session 双跑；running 转移失败必须
// 中止启动（此前 Warn-continue 等于放任并发写者）。

// countedKeyedLock 数 acquire/release，内部是真互斥锁（并发用例会真实串行化）。
type countedKeyedLock struct {
	mu       sync.Mutex
	countMu  sync.Mutex
	acquires int
	releases int
}

func newCountedKeyedLock() *countedKeyedLock { return &countedKeyedLock{} }

func (l *countedKeyedLock) lockFn(_ string) func() {
	l.mu.Lock()
	l.countMu.Lock()
	l.acquires++
	l.countMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.countMu.Lock()
			l.releases++
			l.countMu.Unlock()
			l.mu.Unlock()
		})
	}
}

func (l *countedKeyedLock) counts() (int, int) {
	l.countMu.Lock()
	defer l.countMu.Unlock()
	return l.acquires, l.releases
}

// blockingTeamRunner 在 RunTurnFromInput 内阻塞直到 release 关闭，用于观测
// 「运行中」窗口内的注册表可见性与取消传播。
type blockingTeamRunner struct {
	capturingTeamRunner
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	capturedCtx context.Context
}

func newBlockingTeamRunner() *blockingTeamRunner {
	return &blockingTeamRunner{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingTeamRunner) RunTurnFromInput(ctx context.Context, sess biz.Session, in biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	r.capturedCtx = ctx
	r.startOnce.Do(func() { close(r.started) })
	<-r.release
	r.calls++
	return biz.ChatMessage{ID: "u-1"}, biz.ChatMessage{ID: "a-1"}, nil
}

// cfgOverrideSpiritController 覆盖 GetParallelConfig，其余沿用 stub 默认。
type cfgOverrideSpiritController struct {
	stubSpiritTeamController
	cfg biz.ParallelConfig
}

func (c *cfgOverrideSpiritController) GetParallelConfig(context.Context, string) biz.ParallelConfig {
	return c.cfg
}

// failUpdateSessionRepo 让 UpdateSession 恒失败（会话 running 转移中止场景）。
type failUpdateSessionRepo struct {
	*f10SessionRepo
	err error
}

func (r *failUpdateSessionRepo) UpdateSession(context.Context, string, biz.SessionUpdateFields) (biz.Session, error) {
	return biz.Session{}, r.err
}

func newGuardedStartTurnStarter(sess biz.Session, team biz.Team, runner biz.TeamRunnerWirePort,
	controller *stubSpiritTeamController, writer *stubTeamWriter, guard *TeamRunGuard) *TeamStarter {
	s := newStartTurnTestStarter(sess, team, runner, controller, writer)
	s.SetRunGuard(guard)
	return s
}

// TestStartTeamTurn_BusyRegistrySkips：注册表已有活跃 run（chat 入口占用）
// 时 StartTeamTurn 必须 nil-skip（INV-M4：重复派单不是错误），不动 session/
// team，不跑 runner；锁获取/释放成对。
func TestStartTeamTurn_BusyRegistrySkips(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-g1", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
	sess := biz.Session{
		ID: "team-sess-g1", TeamID: team.ID, ParentSessionID: "spirit-g1",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := &capturingTeamRunner{}
	lock := newCountedKeyedLock()
	runs := rt.NewRunRegistry()
	// 模拟 chat 入口已注册运行实体（跨入口可见性前提）。
	runs.StoreCancelable(sess.ID, "chat-run-1", func() {})
	s := newGuardedStartTurnStarter(sess, team, runner, controller, writer,
		&TeamRunGuard{Runs: runs, Lock: lock.lockFn})

	if err := s.StartTeamTurn(context.Background(), sess.ID, "执行任务"); err != nil {
		t.Fatalf("busy skip must return nil (INV-M4), got %v", err)
	}
	if runner.calls != 0 {
		t.Errorf("RunTurnFromInput calls = %d, want 0 (busy registry skip)", runner.calls)
	}
	if got := writer.snapshotTransitions(); len(got) != 0 {
		t.Errorf("team transitions = %v, want none (skip before any write)", got)
	}
	acq, rel := lock.counts()
	if acq != 1 || rel != 1 {
		t.Errorf("lock acquire/release = %d/%d, want 1/1 (unlock before return)", acq, rel)
	}
	if !runs.HasActive(sess.ID) {
		t.Error("chat run entry must remain (skip must not touch registry)")
	}
}

// TestStartTeamTurn_TransitionRunningErrorAborts：会话 → running 转移失败
// 必须中止启动并返回错误（此前 Warn-continue = 放任并发写者双跑）。
func TestStartTeamTurn_TransitionRunningErrorAborts(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-g2", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
	sess := biz.Session{
		ID: "team-sess-g2", TeamID: team.ID, ParentSessionID: "spirit-g2",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := &capturingTeamRunner{}
	lock := newCountedKeyedLock()
	runs := rt.NewRunRegistry()

	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    writer,
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	repo := &failUpdateSessionRepo{f10SessionRepo: &f10SessionRepo{sessions: []biz.Session{sess}}, err: errors.New("db down")}
	s := &TeamStarter{
		sessions: biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller, TeamsNative: runner},
		lg:       loggateway.NewNoop(),
		runGuard: &TeamRunGuard{Runs: runs, Lock: lock.lockFn},
	}

	err := s.StartTeamTurn(context.Background(), sess.ID, "执行任务")
	if err == nil {
		t.Fatal("session → running transition failure must abort StartTeamTurn with error")
	}
	if runner.calls != 0 {
		t.Errorf("RunTurnFromInput calls = %d, want 0 (abort before run)", runner.calls)
	}
	if runs.HasActive(sess.ID) {
		t.Error("registry must stay empty on abort (registration happens after transition)")
	}
	if got := writer.snapshotTransitions(); len(got) != 0 {
		t.Errorf("team transitions = %v, want none (abort before team write)", got)
	}
	acq, rel := lock.counts()
	if acq != 1 || rel != 1 {
		t.Errorf("lock acquire/release = %d/%d, want 1/1 (defer unlock on abort)", acq, rel)
	}
}

// TestStartTeamTurn_RegistersCancelableAndFinishes：运行窗口内注册表可见、
// turnCtx 可取消；返回后注册释放并且 cancel 已触发（goroutine 泄漏兜底）。
func TestStartTeamTurn_RegistersCancelableAndFinishes(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-g3", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
	sess := biz.Session{
		ID: "team-sess-g3", TeamID: team.ID, ParentSessionID: "spirit-g3",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &cfgOverrideSpiritController{cfg: biz.ParallelConfig{TeamMaxLifetimeSeconds: 3600}}
	runner := newBlockingTeamRunner()
	lock := newCountedKeyedLock()
	runs := rt.NewRunRegistry()
	s := newGuardedStartTurnStarter(sess, team, runner, &controller.stubSpiritTeamController, writer,
		&TeamRunGuard{Runs: runs, Lock: lock.lockFn})
	// 用覆盖版 controller 替换（GetParallelConfig 提供超时，cancel 可观测）。
	s.team.SpiritUC = controller

	done := make(chan error, 1)
	go func() { done <- s.StartTeamTurn(context.Background(), sess.ID, "执行任务") }()

	<-runner.started
	if !runs.HasActive(sess.ID) {
		t.Fatal("registry must expose the in-flight run while runner is blocked")
	}
	if runner.capturedCtx.Err() != nil {
		t.Fatalf("turn ctx cancelled while runner still blocked: %v", runner.capturedCtx.Err())
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("StartTeamTurn: %v", err)
	}

	if runs.HasActive(sess.ID) {
		t.Error("registry entry must be finished after StartTeamTurn returns")
	}
	if runner.capturedCtx.Err() != context.Canceled {
		t.Errorf("turn ctx err = %v, want context.Canceled (deferred cancel)", runner.capturedCtx.Err())
	}
	acq, rel := lock.counts()
	if acq != 1 || rel != 1 {
		t.Errorf("lock acquire/release = %d/%d, want 1/1", acq, rel)
	}
}

// TestStartTeamTurn_ConcurrentDispatchSerialize：并发 dispatch 同一 team
// session 至多一个进入 runner（先注册者赢，后来者 busy skip nil）。
func TestStartTeamTurn_ConcurrentDispatchSerialize(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-g4", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
	sess := biz.Session{
		ID: "team-sess-g4", TeamID: team.ID, ParentSessionID: "spirit-g4",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := newBlockingTeamRunner()
	lock := newCountedKeyedLock()
	runs := rt.NewRunRegistry()
	s := newGuardedStartTurnStarter(sess, team, runner, controller, writer,
		&TeamRunGuard{Runs: runs, Lock: lock.lockFn})

	first := make(chan error, 1)
	go func() { first <- s.StartTeamTurn(context.Background(), sess.ID, "第一轮") }()
	<-runner.started // 第一位已进入 runner，注册表活跃

	second := make(chan error, 1)
	go func() { second <- s.StartTeamTurn(context.Background(), sess.ID, "第二轮") }()
	if err := <-second; err != nil {
		t.Fatalf("second dispatch must nil-skip on busy registry, got %v", err)
	}
	close(runner.release)
	if err := <-first; err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if runner.calls != 1 {
		t.Errorf("RunTurnFromInput calls = %d, want exactly 1 (serialized dispatch)", runner.calls)
	}
	if runs.HasActive(sess.ID) {
		t.Error("registry entry must be finished after both dispatches settle")
	}
}

// TestStartTeamTurn_NilGuard_LegacyPath：guard 为 nil（v1-only/单测）时走
// legacy 路径，但 running 转移失败的中止语义无例外生效。
func TestStartTeamTurn_NilGuard_LegacyPath(t *testing.T) {
	t.Run("SuccessRunsRunner", func(t *testing.T) {
		writer := &stubTeamWriter{}
		team := biz.Team{ID: "team-g5", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
		sess := biz.Session{
			ID: "team-sess-g5", TeamID: team.ID, ParentSessionID: "spirit-g5",
			Status: string(sessstatus.SessionStatusIdle),
		}
		controller := &stubSpiritTeamController{}
		runner := &capturingTeamRunner{}
		s := newStartTurnTestStarter(sess, team, runner, controller, writer)

		if err := s.StartTeamTurn(context.Background(), sess.ID, "执行任务"); err != nil {
			t.Fatalf("legacy path success: %v", err)
		}
		if runner.calls != 1 {
			t.Errorf("RunTurnFromInput calls = %d, want 1", runner.calls)
		}
	})
	t.Run("TransitionErrorStillAborts", func(t *testing.T) {
		writer := &stubTeamWriter{}
		team := biz.Team{ID: "team-g6", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
		sess := biz.Session{
			ID: "team-sess-g6", TeamID: team.ID, ParentSessionID: "spirit-g6",
			Status: string(sessstatus.SessionStatusIdle),
		}
		controller := &stubSpiritTeamController{}
		runner := &capturingTeamRunner{}
		teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
			Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
			Writer:    writer,
			RunReader: &stubTeamRunReader{},
			Lg:        loggateway.NewNoop(),
		})
		repo := &failUpdateSessionRepo{f10SessionRepo: &f10SessionRepo{sessions: []biz.Session{sess}}, err: errors.New("db down")}
		s := &TeamStarter{
			sessions: biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
			team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller, TeamsNative: runner},
			lg:       loggateway.NewNoop(),
		}
		if err := s.StartTeamTurn(context.Background(), sess.ID, "执行任务"); err == nil {
			t.Fatal("guard=nil 也必须中止：running 转移失败不再 Warn-continue")
		}
		if runner.calls != 0 {
			t.Errorf("RunTurnFromInput calls = %d, want 0", runner.calls)
		}
	})
}
