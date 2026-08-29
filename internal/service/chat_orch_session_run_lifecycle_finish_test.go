package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// finishSessionRunRepoStub is a phase-tracking SessionRunRepo for
// FinishSessionRunLifecycle tests: CAS-applies transitions and records calls.
type finishSessionRunRepoStub struct {
	run                biz.SessionRun
	markTerminalCalls  int
	transitionCalls    int
	transitionFromPhase string
	transitionToPhase   string
}

func (s *finishSessionRunRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	if s.run.ID == id {
		return s.run, nil
	}
	return biz.SessionRun{}, nil
}
func (s *finishSessionRunRepoStub) GetActiveForSession(context.Context, string) (biz.SessionRun, error) {
	return biz.SessionRun{}, nil
}
func (s *finishSessionRunRepoStub) ListBySession(context.Context, string, int, int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *finishSessionRunRepoStub) ListForJobs(context.Context, biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *finishSessionRunRepoStub) ListByPhase(context.Context, string, int) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *finishSessionRunRepoStub) Create(context.Context, biz.SessionRun) (string, error) {
	return "", nil
}
func (s *finishSessionRunRepoStub) UpdateCheckpointID(context.Context, string, string) error {
	return nil
}
func (s *finishSessionRunRepoStub) MarkTerminal(context.Context, string, string, string) error {
	return nil
}
func (s *finishSessionRunRepoStub) MarkTerminalWherePhase(_ context.Context, id, fromPhase, toPhase, errMsg string) (bool, error) {
	s.markTerminalCalls++
	if s.run.ID != id || s.run.Phase != fromPhase {
		return false, nil
	}
	s.run.Phase = toPhase
	s.run.ErrorMessage = errMsg
	return true, nil
}
func (s *finishSessionRunRepoStub) TryClaimDurableResume(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *finishSessionRunRepoStub) ClearResumeClaim(context.Context, string) error { return nil }
func (s *finishSessionRunRepoStub) MarkOrphanedRunsCancelled(context.Context) (int, error) {
	return 0, nil
}
func (s *finishSessionRunRepoStub) TransitionPhase(_ context.Context, id, fromPhase, toPhase string) (bool, error) {
	s.transitionCalls++
	s.transitionFromPhase = fromPhase
	s.transitionToPhase = toPhase
	if s.run.ID != id || s.run.Phase != fromPhase {
		return false, nil
	}
	s.run.Phase = toPhase
	return true, nil
}

func newFinishTestLifecycle(repo *finishSessionRunRepoStub, reg *rt.RunRegistry) *chatSessionRunLifecycle {
	return newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns: biz.NewSessionRunUsecase(repo, nil, loggateway.NewNoop()),
		RunStatus:   noopRunStatusTracker{},
		Runs:        reg,
		Logger:      loggateway.NewNoop(),
	})
}

// N1（session-eval-20260827 S10 / C5-①）：取消 run 的 session_runs 行 phase
// 必须落 cancelled，与 turns_v2/API 三方一致——修复前 Finish 以 turnErr==nil
// 走 Complete 落 completed（成功率虚高），或以 context.Canceled 走 Fail 落
// failed。三个取消信号（注册表状态 / turnErr / ctx）逐一钉住。
func TestFinishSessionRunLifecycle_CancelLandsCancelled(t *testing.T) {
	cases := []struct {
		name      string
		setupReg  bool // registry 预置 cancelled 状态
		turnErr   error
		cancelCtx bool
	}{
		{"registry cancelled status", true, nil, false},
		{"turnErr context.Canceled", false, context.Canceled, false},
		{"ctx cancelled", false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &finishSessionRunRepoStub{run: biz.SessionRun{
				ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
			}}
			reg := rt.NewRunRegistry()
			if tc.setupReg {
				reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
			}
			lc := newFinishTestLifecycle(repo, reg)
			ctx := context.Background()
			if tc.cancelCtx {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			lc.FinishSessionRunLifecycle(ctx, "sess-1", "srid-1", tc.turnErr)
			if repo.run.Phase != biz.SessionRunPhaseCancelled {
				t.Fatalf("phase=%s, want cancelled", repo.run.Phase)
			}
			if repo.markTerminalCalls != 0 {
				t.Fatalf("MarkTerminalWherePhase must not be called on cancel path, got %d", repo.markTerminalCalls)
			}
		})
	}
}

// durable 交棒防回归：applyDurableTransition 自身会 runs.Cancel +
// SetRunStatus(cancelled) 作为交棒信号；Finish 不得把 durable 行误迁
// cancelled（否则长效任务永不被 resume）。
func TestFinishSessionRunLifecycle_DurableHandoffNotMisreadAsCancel(t *testing.T) {
	repo := &finishSessionRunRepoStub{run: biz.SessionRun{
		ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseDurable,
	}}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "durable_escalate")
	lc := newFinishTestLifecycle(repo, reg)
	lc.FinishSessionRunLifecycle(context.Background(), "sess-1", "srid-1", nil)
	if repo.run.Phase != biz.SessionRunPhaseDurable {
		t.Fatalf("durable row must survive Finish, got phase=%s", repo.run.Phase)
	}
}

// 终态幂等：行已被并发路径写终态时 Finish 不覆写、不再发起终态迁移（原
// Error 日志属误导性终态竞态）。
func TestFinishSessionRunLifecycle_TerminalIdempotent(t *testing.T) {
	repo := &finishSessionRunRepoStub{run: biz.SessionRun{
		ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseCompleted,
	}}
	lc := newFinishTestLifecycle(repo, rt.NewRunRegistry())
	lc.FinishSessionRunLifecycle(context.Background(), "sess-1", "srid-1", nil)
	if repo.run.Phase != biz.SessionRunPhaseCompleted {
		t.Fatalf("terminal row must not be overwritten, got phase=%s", repo.run.Phase)
	}
	if repo.markTerminalCalls != 0 || repo.transitionCalls != 0 {
		t.Fatalf("no transition expected on terminal row, got mark=%d trans=%d",
			repo.markTerminalCalls, repo.transitionCalls)
	}
}

// 正常完成/失败路径防退化：无取消信号时行为与修复前一致。
func TestFinishSessionRunLifecycle_CompleteAndFailUnchanged(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		repo := &finishSessionRunRepoStub{run: biz.SessionRun{
			ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
		}}
		lc := newFinishTestLifecycle(repo, rt.NewRunRegistry())
		lc.FinishSessionRunLifecycle(context.Background(), "sess-1", "srid-1", nil)
		if repo.run.Phase != biz.SessionRunPhaseCompleted {
			t.Fatalf("phase=%s, want completed", repo.run.Phase)
		}
	})
	t.Run("fail", func(t *testing.T) {
		repo := &finishSessionRunRepoStub{run: biz.SessionRun{
			ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
		}}
		lc := newFinishTestLifecycle(repo, rt.NewRunRegistry())
		lc.FinishSessionRunLifecycle(context.Background(), "sess-1", "srid-1", errors.New("boom"))
		if repo.run.Phase != biz.SessionRunPhaseFailed {
			t.Fatalf("phase=%s, want failed", repo.run.Phase)
		}
		if repo.run.ErrorMessage != "boom" {
			t.Fatalf("errMsg=%q, want boom", repo.run.ErrorMessage)
		}
	})
}

func TestFinishSessionRunLifecycle_AwaitingUserDoesNotComplete(t *testing.T) {
	repo := &finishSessionRunRepoStub{run: biz.SessionRun{
		ID: "srid-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", string(biz.RunStateAwaitingUser), "")
	lc := newFinishTestLifecycle(repo, reg)
	lc.FinishSessionRunLifecycle(context.Background(), "sess-1", "srid-1", nil)
	if repo.run.Phase != biz.SessionRunPhaseInteractive {
		t.Fatalf("awaiting_user must keep session_runs interactive, got %s", repo.run.Phase)
	}
}

// G2 续跑认领（2026-08-29 审查发现）：挂起（awaiting_user）的 session_runs
// 行保持 interactive，续跑 turn 的 Begin 撞 StartInteractive 并发守卫后
// 必须认领既有 run，而非静默丢绑定。
func TestBeginSessionRunLifecycle_AdoptsAwaitingRun(t *testing.T) {
	repo := &adoptSessionRunRepoStub{active: biz.SessionRun{
		ID: "srid-await", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", string(biz.RunStateAwaitingUser), "")
	lc := newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns: biz.NewSessionRunUsecase(repo, nil, loggateway.NewNoop()),
		RunStatus:   noopRunStatusTracker{},
		Runs:        reg,
		Logger:      loggateway.NewNoop(),
	})
	_, runID := lc.BeginSessionRunLifecycle(context.Background(), SessionRunStartParams{
		Session: biz.Session{ID: "sess-1"},
		TurnID:  "turn-2",
	})
	if runID != "srid-await" {
		t.Fatalf("expected adopted run srid-await, got %q", runID)
	}
	if repo.createCalls != 0 {
		t.Fatalf("adoption must not create a new row, createCalls=%d", repo.createCalls)
	}
}

// 非挂起会话的真冲突仍 fail closed（不认领、不建行）。
func TestBeginSessionRunLifecycle_GenuineConflictFailsClosed(t *testing.T) {
	repo := &adoptSessionRunRepoStub{active: biz.SessionRun{
		ID: "srid-busy", SessionID: "sess-1", Phase: biz.SessionRunPhaseInteractive,
	}}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", "running", "")
	lc := newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns: biz.NewSessionRunUsecase(repo, nil, loggateway.NewNoop()),
		RunStatus:   noopRunStatusTracker{},
		Runs:        reg,
		Logger:      loggateway.NewNoop(),
	})
	_, runID := lc.BeginSessionRunLifecycle(context.Background(), SessionRunStartParams{
		Session: biz.Session{ID: "sess-1"},
		TurnID:  "turn-2",
	})
	if runID != "" {
		t.Fatalf("genuine conflict must not adopt, got %q", runID)
	}
	if repo.createCalls != 0 {
		t.Fatalf("conflict must not create a new row, createCalls=%d", repo.createCalls)
	}
}

// adoptSessionRunRepoStub simulates a session with an active interactive run:
// Create reports the DB unique-index conflict that StartInteractive's
// pre-flight GetActiveForSession turns into apierror.Conflict.
type adoptSessionRunRepoStub struct {
	active      biz.SessionRun
	createCalls int
}

func (s *adoptSessionRunRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	if s.active.ID == id {
		return s.active, nil
	}
	return biz.SessionRun{}, apierror.NotFound(apierror.DomainSessionRun, "not found")
}
func (s *adoptSessionRunRepoStub) GetActiveForSession(_ context.Context, sessionID string) (biz.SessionRun, error) {
	if s.active.SessionID == sessionID && s.active.Phase == biz.SessionRunPhaseInteractive {
		return s.active, nil
	}
	return biz.SessionRun{}, apierror.NotFound(apierror.DomainSessionRun, "not found")
}
func (s *adoptSessionRunRepoStub) ListBySession(context.Context, string, int, int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *adoptSessionRunRepoStub) ListForJobs(context.Context, biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *adoptSessionRunRepoStub) ListByPhase(context.Context, string, int) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *adoptSessionRunRepoStub) Create(context.Context, biz.SessionRun) (string, error) {
	s.createCalls++
	return "", nil
}
func (s *adoptSessionRunRepoStub) UpdateCheckpointID(context.Context, string, string) error {
	return nil
}
func (s *adoptSessionRunRepoStub) MarkTerminal(context.Context, string, string, string) error {
	return nil
}
func (s *adoptSessionRunRepoStub) MarkTerminalWherePhase(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (s *adoptSessionRunRepoStub) TryClaimDurableResume(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *adoptSessionRunRepoStub) ClearResumeClaim(context.Context, string) error { return nil }
func (s *adoptSessionRunRepoStub) MarkOrphanedRunsCancelled(context.Context) (int, error) {
	return 0, nil
}
func (s *adoptSessionRunRepoStub) TransitionPhase(context.Context, string, string, string) (bool, error) {
	return false, nil
}
