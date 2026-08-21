package team

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P0 终态一致性：ReconcileStaleRuns 收割丢失的 running team run。
// 覆盖：收割丢失 run、跳过 waiting_human/活跃 run、终态 run 残留会话清理。

func registerReconcileTestSession(t *testing.T, coord *TeamGraphRunCoordinator, execID, runID string) {
	t.Helper()
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "member-1", Type: "agent", AgentName: "a1"}}}, nil, nil, nil)
	if err := coord.RegisterTeamGraphExecution(context.Background(), execID, "sess-1", "sess-1", "team-1", runID, "", ct); err != nil {
		t.Fatal(err)
	}
}

func TestTeamGraphRunCoordinator_ReconcileStaleRuns_ReapsLostRun(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning},
	}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	registerReconcileTestSession(t, coord, "exec-1", "run-1")

	now := time.Now()
	coord.sessions["exec-1"].lastActivityAt = now.Add(-2 * time.Hour)

	var gotTeam, gotRun, gotReason string
	calls := 0
	coord.SetStaleRunHandler(func(teamID, teamRunID, reason string) {
		calls++
		gotTeam, gotRun, gotReason = teamID, teamRunID, reason
	})

	if n := coord.ReconcileStaleRuns(context.Background(), now, time.Minute); n != 1 {
		t.Fatalf("reconciled=%d want=1", n)
	}
	run, err := repo.GetTeamRunByID(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != biz.TeamRunStatusFailed {
		t.Fatalf("run status=%s want=%s", run.Status, biz.TeamRunStatusFailed)
	}
	if !strings.Contains(run.ErrorMessage, "stale run reconciled") {
		t.Fatalf("error_message=%q want contains 'stale run reconciled'", run.ErrorMessage)
	}
	if coord.session("exec-1") != nil {
		t.Fatal("session still resident after reconcile")
	}
	if _, err := sessRepo.GetSession(context.Background(), "exec-1"); err == nil {
		t.Fatal("session row still in DB after reconcile")
	}
	if calls != 1 || gotTeam != "team-1" || gotRun != "run-1" || !strings.Contains(gotReason, "stale run reconciled") {
		t.Fatalf("handler calls=%d team=%q run=%q reason=%q", calls, gotTeam, gotRun, gotReason)
	}
}

func TestTeamGraphRunCoordinator_ReconcileStaleRuns_SkipsWaitingHumanAndActive(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-hitl":   {ID: "run-hitl", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman},
		"run-active": {ID: "run-active", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning},
	}}
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	registerReconcileTestSession(t, coord, "exec-hitl", "run-hitl")
	registerReconcileTestSession(t, coord, "exec-active", "run-active")

	now := time.Now()
	// HITL 会话：超时但属 SLA 路径，不得收割。
	coord.sessions["exec-hitl"].status = biz.TeamRunStatusWaitingHuman
	coord.sessions["exec-hitl"].lastActivityAt = now.Add(-2 * time.Hour)
	// 活跃 running 会话：刚刚有活动，不得收割。
	coord.sessions["exec-active"].lastActivityAt = now

	calls := 0
	coord.SetStaleRunHandler(func(teamID, teamRunID, reason string) { calls++ })

	if n := coord.ReconcileStaleRuns(context.Background(), now, time.Minute); n != 0 {
		t.Fatalf("reconciled=%d want=0", n)
	}
	if coord.session("exec-hitl") == nil || coord.session("exec-active") == nil {
		t.Fatal("waiting_human / active session must stay resident")
	}
	for _, id := range []string{"run-hitl", "run-active"} {
		run, err := repo.GetTeamRunByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if biz.IsTeamRunTerminalStatus(run.Status) {
			t.Fatalf("%s unexpectedly terminal: %s", id, run.Status)
		}
	}
	if calls != 0 {
		t.Fatalf("handler called %d times, want 0", calls)
	}
}

func TestTeamGraphRunCoordinator_ReconcileStaleRuns_AlreadyTerminalEvictsWithoutNotify(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusSuccess},
	}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	registerReconcileTestSession(t, coord, "exec-1", "run-1")

	now := time.Now()
	coord.sessions["exec-1"].lastActivityAt = now.Add(-2 * time.Hour)

	calls := 0
	coord.SetStaleRunHandler(func(teamID, teamRunID, reason string) { calls++ })

	// 已终态 run 的残留会话：仅驱逐，不重复计收割、不发失败通知。
	if n := coord.ReconcileStaleRuns(context.Background(), now, time.Minute); n != 0 {
		t.Fatalf("reconciled=%d want=0", n)
	}
	if coord.session("exec-1") != nil {
		t.Fatal("residual session of terminal run must be evicted")
	}
	if _, err := sessRepo.GetSession(context.Background(), "exec-1"); err == nil {
		t.Fatal("session row still in DB after residual eviction")
	}
	run, err := repo.GetTeamRunByID(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("terminal run status mutated: %s", run.Status)
	}
	if calls != 0 {
		t.Fatalf("handler called %d times for terminal run, want 0", calls)
	}
}