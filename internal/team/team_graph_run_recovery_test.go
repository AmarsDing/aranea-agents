package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 83-长时运行韧性：RecoverSessions 崩溃续跑测试（从
// team_graph_run_coordinator_test.go 拆出，AS-COG-01 行数纪律）。

func TestTeamGraphRunCoordinator_RecoverSessions(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman, DefinitionSnapshotJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`},
		"run-2": {ID: "run-2", TeamID: "team-1", SessionID: "sess-2", Status: biz.TeamRunStatusRunning},
	}}
	sessRepo := newMemSessionRepo()
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:         "exec-1",
		TeamRunID:      "run-1",
		TeamID:         "team-1",
		SessionID:      "sess-1",
		Status:         biz.TeamRunStatusWaitingHuman,
		DefinitionJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`,
	})
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:    "exec-2",
		TeamRunID: "run-2",
		TeamID:    "team-1",
		SessionID: "sess-2",
		Status:    biz.TeamRunStatusRunning,
	})

	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	coord.RecoverSessions(context.Background())

	sess := coord.session("exec-1")
	if sess == nil {
		t.Fatal("waiting_human session should be recovered")
	}
	if sess.teamRunID != "run-1" {
		t.Fatalf("expected run-1, got %q", sess.teamRunID)
	}
	sess2 := coord.session("exec-2")
	if sess2 != nil {
		t.Fatal("running session should not be recovered (orphaned)")
	}
	// 83: crash resume attempted but backend has no checkpoint → run failed, no marker.
	if coord.WasStartupResumed("run-2") {
		t.Fatal("failed crash resume must not set startup-resumed marker")
	}
	run2, err := repo.GetTeamRunByID(context.Background(), "run-2")
	if err != nil {
		t.Fatalf("run-2 must remain readable after finalize: %v", err)
	}
	if run2.Status != biz.TeamRunStatusFailed {
		t.Fatalf("run-2 status=%q, want failed after crash resume rejection", run2.Status)
	}
}

// crashResumeStubBackend overrides crash-resume outcome for RecoverSessions tests.
type crashResumeStubBackend struct {
	inner   TeamGraphExecutionBackend
	recover func(ctx context.Context, executionID string) (*biz.GraphExecution, error)
}

func (b *crashResumeStubBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}
func (b *crashResumeStubBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *crashResumeStubBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.inner.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}
func (b *crashResumeStubBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.inner.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
func (b *crashResumeStubBackend) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error) {
	return b.inner.ResumeExecution(ctx, executionID, resumeValue)
}
func (b *crashResumeStubBackend) RecoverOrphanedExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.recover(ctx, executionID)
}
func (b *crashResumeStubBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}

// 83-长时运行韧性：running 会话从 checkpoint 续跑成功 → 内存会话重建 +
// startupResumed 命中（team 级判死跳过依据），run 不被判死。
func TestTeamGraphRunCoordinator_RecoverSessions_CrashResumeSuccess(t *testing.T) {
	inner := newCoordTestBackend()
	backend := &crashResumeStubBackend{
		inner: inner,
		recover: func(_ context.Context, executionID string) (*biz.GraphExecution, error) {
			return &biz.GraphExecution{ID: executionID, Status: biz.TeamRunStatusRunning}, nil
		},
	}
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning, DefinitionSnapshotJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`},
	}}
	sessRepo := newMemSessionRepo()
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:         "exec-1",
		TeamRunID:      "run-1",
		TeamID:         "team-1",
		SessionID:      "sess-1",
		Status:         biz.TeamRunStatusRunning,
		DefinitionJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`,
	})

	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	coord.RecoverSessions(context.Background())

	if coord.session("exec-1") == nil {
		t.Fatal("crash-resumed running session must be re-registered in memory")
	}
	if !coord.WasStartupResumed("run-1") {
		t.Fatal("startup-resumed marker must be set for successfully resumed run")
	}
	run, err := repo.GetTeamRunByID(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != biz.TeamRunStatusRunning {
		t.Fatalf("run-1 status=%q, want running untouched", run.Status)
	}
}

// 83-长时运行韧性：开关关闭时 running 会话不走续跑，由 MarkOrphanedSessionsTerminal 兜底。
func TestTeamGraphRunCoordinator_RecoverSessions_CrashResumeDisabled(t *testing.T) {
	called := false
	inner := newCoordTestBackend()
	backend := &crashResumeStubBackend{
		inner: inner,
		recover: func(_ context.Context, executionID string) (*biz.GraphExecution, error) {
			called = true
			return &biz.GraphExecution{ID: executionID, Status: biz.TeamRunStatusRunning}, nil
		},
	}
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{}}
	sessRepo := newMemSessionRepo()
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:    "exec-1",
		TeamRunID: "run-1",
		TeamID:    "team-1",
		SessionID: "sess-1",
		Status:    biz.TeamRunStatusRunning,
	})

	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	coord.SetCrashResumeEnabled(false)
	coord.RecoverSessions(context.Background())

	if called {
		t.Fatal("RecoverOrphanedExecution must not be called when crash resume disabled")
	}
	if coord.session("exec-1") != nil {
		t.Fatal("running session must not be re-registered when crash resume disabled")
	}
	if coord.WasStartupResumed("run-1") {
		t.Fatal("marker must not be set when crash resume disabled")
	}
}
