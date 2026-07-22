package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// escalateAllSessionRunRepoStub implements biz.SessionRunRepo for the
// EscalateAllActiveToDurable test.
type escalateAllSessionRunRepoStub struct {
	biz.SessionRunRepo
	runs    []biz.SessionRun
	listErr error
}

func (s *escalateAllSessionRunRepoStub) ListByPhase(_ context.Context, _ string, _ int) ([]biz.SessionRun, error) {
	return s.runs, s.listErr
}

// spySessionRunLifecycle records EscalateToDurableOnShutdown invocations.
type spySessionRunLifecycle struct {
	noopSessionRunLifecycle
	calls []string
}

func (s *spySessionRunLifecycle) EscalateToDurableOnShutdown(_ context.Context, sessionID, sessionRunID string) {
	s.calls = append(s.calls, sessionID+"/"+sessionRunID)
}

func newEscalateAllTestService(runs []biz.SessionRun, listErr error, spy *spySessionRunLifecycle) *ChatService {
	orch := &ChatOrchestrator{
		channelDeps: ChatChannelDeps{
			ChJobs: ChannelTurnJobDeps{
				SessionRuns: biz.NewSessionRunUsecase(&escalateAllSessionRunRepoStub{runs: runs, listErr: listErr}, nil, nil),
			},
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    noopRunStatusTracker{},
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: spy,
		},
	}
	return &ChatService{lg: loggateway.NewNoop(), orch: orch}
}

// L2 (2026-07-22)：shutdown 时所有 interactive run 必须被 durable 化，
// 重启后由 SessionRunDurableWorker 自动续跑。
func TestEscalateAllActiveToDurable_happy(t *testing.T) {
	spy := &spySessionRunLifecycle{}
	svc := newEscalateAllTestService([]biz.SessionRun{
		{ID: "run-1", SessionID: "sess-1"},
		{ID: "run-2", SessionID: "sess-2"},
		{ID: "", SessionID: "sess-3"},  // missing ID: skipped
		{ID: "run-4", SessionID: ""},   // missing session: skipped
	}, nil, spy)

	if got := svc.EscalateAllActiveToDurable(context.Background()); got != 2 {
		t.Fatalf("escalated=%d, want 2 (entries with empty ID/SessionID skipped)", got)
	}
	if len(spy.calls) != 2 {
		t.Fatalf("EscalateToDurableOnShutdown calls=%v, want 2 entries", spy.calls)
	}
	if spy.calls[0] != "sess-1/run-1" || spy.calls[1] != "sess-2/run-2" {
		t.Fatalf("calls=%v", spy.calls)
	}
}

// ListByPhase 失败不得 panic，返回 0（best-effort shutdown path）。
func TestEscalateAllActiveToDurable_listError(t *testing.T) {
	spy := &spySessionRunLifecycle{}
	svc := newEscalateAllTestService(nil, errors.New("db down"), spy)
	if got := svc.EscalateAllActiveToDurable(context.Background()); got != 0 {
		t.Fatalf("escalated=%d, want 0", got)
	}
	if len(spy.calls) != 0 {
		t.Fatalf("calls=%v, want none", spy.calls)
	}
}

// nil 依赖安全（构造不全的 service 不得 panic）。
func TestEscalateAllActiveToDurable_nilSafe(t *testing.T) {
	var nilSvc *ChatService
	if got := nilSvc.EscalateAllActiveToDurable(context.Background()); got != 0 {
		t.Fatalf("nil service escalated=%d, want 0", got)
	}
	empty := &ChatService{lg: loggateway.NewNoop()}
	if got := empty.EscalateAllActiveToDurable(context.Background()); got != 0 {
		t.Fatalf("no-orch escalated=%d, want 0", got)
	}
}
