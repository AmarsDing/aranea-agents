package biz

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// orphanRecoveryRepo records run-level CAS calls so tests can assert which
// runs RecoverOrphanedRunningTeams attempted to fail.
type orphanRecoveryRepo struct {
	stubTeamReader
	stubTeamWriter
	stubTeamRunReader
	stubTeamRunWriter

	mu           sync.Mutex
	teams        []Team
	runs         []TeamRunRecord
	casRunStatus map[string]string
}

func (r *orphanRecoveryRepo) ListTeamsByStatus(_ context.Context, status string) ([]Team, error) {
	var out []Team
	for _, t := range r.teams {
		if t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *orphanRecoveryRepo) GetTeamByID(_ context.Context, id string) (Team, error) {
	for _, t := range r.teams {
		if t.ID == id {
			return t, nil
		}
	}
	return Team{}, ErrNotFound
}

func (r *orphanRecoveryRepo) UpdateTeamWhereStatus(_ context.Context, id, newStatus, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.teams {
		if r.teams[i].ID == id {
			r.teams[i].Status = newStatus
			return true, nil
		}
	}
	return false, nil
}

func (r *orphanRecoveryRepo) UpdateTeam(_ context.Context, t Team) (Team, error) { return t, nil }

func (r *orphanRecoveryRepo) ListTeamRuns(_ context.Context, teamID string, _ int) ([]TeamRunRecord, error) {
	var out []TeamRunRecord
	for _, run := range r.runs {
		if run.TeamID == teamID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (r *orphanRecoveryRepo) GetTeamRunByID(_ context.Context, id string) (TeamRunRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.ID == id {
			return run, nil
		}
	}
	return TeamRunRecord{}, ErrNotFound
}

func (r *orphanRecoveryRepo) UpdateTeamRunWhereStatus(_ context.Context, runID, newStatus, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.casRunStatus == nil {
		r.casRunStatus = make(map[string]string)
	}
	r.casRunStatus[runID] = newStatus
	for i := range r.runs {
		if r.runs[i].ID == runID {
			r.runs[i].Status = newStatus
			return true, nil
		}
	}
	return false, nil
}

func (r *orphanRecoveryRepo) casStatusFor(runID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.casRunStatus[runID]
	return s, ok
}

// TestRecoverOrphanedRunningTeams_SkipsWaitingHumanAndPausedRuns is the B2
// regression test: startup orphan recovery must only fail pending/running
// runs. waiting_human runs are owned by the graph-session HITL recovery
// channel (RecoverSessions + completion watch) and must NOT be force-failed,
// otherwise the human verdict after restart would be silently discarded.
func TestRecoverOrphanedRunningTeams_SkipsWaitingHumanAndPausedRuns(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs: []TeamRunRecord{
			{ID: "run-pending", TeamID: "team-1", Status: TeamRunStatusPending},
			{ID: "run-running", TeamID: "team-1", Status: TeamRunStatusRunning},
			{ID: "run-hitl", TeamID: "team-1", Status: TeamRunStatusWaitingHuman},
			{ID: "run-paused", TeamID: "team-1", Status: TeamRunStatusPaused},
			{ID: "run-done", TeamID: "team-1", Status: TeamRunStatusSuccess},
		},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})

	recovered, err := uc.RecoverOrphanedRunningTeams(context.Background())
	if err != nil {
		t.Fatalf("RecoverOrphanedRunningTeams: unexpected error: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered teams = %d, want 1", len(recovered))
	}

	if s, ok := repo.casStatusFor("run-pending"); !ok || s != TeamRunStatusCancelled {
		t.Errorf("run-pending: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusCancelled)
	}
	if s, ok := repo.casStatusFor("run-running"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-running: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
	for _, id := range []string{"run-hitl", "run-paused", "run-done"} {
		if s, ok := repo.casStatusFor(id); ok {
			t.Errorf("run %s (waiting_human/paused/terminal) must NOT be touched by orphan recovery, got CAS to %q", id, s)
		}
	}
}

type stubStartupResumeMarker map[string]bool

func (m stubStartupResumeMarker) WasStartupResumed(runID string) bool { return m[runID] }

// 83-长时运行韧性：team 的活跃 running run 已在启动对账中从 checkpoint
// 续跑成功（marker 命中）→ 整个 team 跳过判死：team 不 interrupted，run 不动。
func TestRecoverOrphanedRunningTeamsEx_SkipsStartupResumedTeam(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{
			{ID: "team-resumed", Status: TeamStatusRunning},
			{ID: "team-orphan", Status: TeamStatusRunning},
		},
		runs: []TeamRunRecord{
			{ID: "run-resumed", TeamID: "team-resumed", Status: TeamRunStatusRunning},
			{ID: "run-orphan", TeamID: "team-orphan", Status: TeamRunStatusRunning},
		},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})
	marker := stubStartupResumeMarker{"run-resumed": true}

	recovered, err := uc.RecoverOrphanedRunningTeamsEx(context.Background(), marker)
	if err != nil {
		t.Fatalf("RecoverOrphanedRunningTeamsEx: unexpected error: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != "team-orphan" {
		t.Fatalf("recovered = %v, want only team-orphan", recovered)
	}
	// marker 命中：team-resumed 与其 run 均不被触碰。
	for _, tm := range repo.teams {
		if tm.ID == "team-resumed" && tm.Status != TeamStatusRunning {
			t.Errorf("team-resumed status = %q, want running untouched", tm.Status)
		}
	}
	if s, ok := repo.casStatusFor("run-resumed"); ok {
		t.Errorf("run-resumed must NOT be killed, got CAS to %q", s)
	}
	// 未命中：走旧判死路径。
	if s, ok := repo.casStatusFor("run-orphan"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-orphan: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
}

// 83-长时运行韧性：nil marker 退化为旧行为（等价 RecoverOrphanedRunningTeams）。
func TestRecoverOrphanedRunningTeamsEx_NilMarkerLegacy(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs:  []TeamRunRecord{{ID: "run-1", TeamID: "team-1", Status: TeamRunStatusRunning}},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})
	if _, err := uc.RecoverOrphanedRunningTeamsEx(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := repo.casStatusFor("run-1"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-1: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
}

// --- Task 7（83）：判死分支 flowlog 审计 ---

type orphanFlowLogCall struct {
	sessionID string
	stepID    string
	pairs     []LogPair
}

type stubOrphanFlowLog struct {
	warns []orphanFlowLogCall
}

func (s *stubOrphanFlowLog) LogFlowStart(context.Context, string, string, string, ...LogPair) {}
func (s *stubOrphanFlowLog) LogFlowDone(context.Context, string, string, string, ...LogPair)  {}
func (s *stubOrphanFlowLog) LogFlowError(context.Context, string, string, string, ...LogPair) {}
func (s *stubOrphanFlowLog) LogFlowWarn(_ context.Context, sid, step, _ string, pairs ...LogPair) {
	s.warns = append(s.warns, orphanFlowLogCall{sid, step, pairs})
}

// 判死分支补 flowlog：每个被终结的 orphan run 恰好一条
// team.run.orphan_finalize（warn），pairs 带 team/run/target 归属；
// waiting_human / 终态 run 不发。
func TestRecoverOrphanedRunningTeamsEx_OrphanFinalizeFlowLog(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs: []TeamRunRecord{
			{ID: "run-pending", TeamID: "team-1", SessionID: "sess-p", Status: TeamRunStatusPending},
			{ID: "run-running", TeamID: "team-1", SessionID: "sess-r", Status: TeamRunStatusRunning},
			{ID: "run-hitl", TeamID: "team-1", SessionID: "sess-h", Status: TeamRunStatusWaitingHuman},
		},
	}
	flowLog := &stubOrphanFlowLog{}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:        repo,
		Writer:        repo,
		RunReader:     repo,
		RunWriter:     repo,
		Lg:            loggateway.NewNoop(),
		FlowLogWriter: flowLog,
	})

	if _, err := uc.RecoverOrphanedRunningTeams(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flowLog.warns) != 2 {
		t.Fatalf("orphan_finalize warns=%d, want 2 (pending+running only)", len(flowLog.warns))
	}
	byRun := map[string]orphanFlowLogCall{}
	for _, w := range flowLog.warns {
		if w.stepID != "team.run.orphan_finalize" {
			t.Fatalf("step_id=%q, want team.run.orphan_finalize", w.stepID)
		}
		for _, p := range w.pairs {
			if p.Key == "run_id" {
				if id, ok := p.Value.(string); ok {
					byRun[id] = w
				}
			}
		}
	}
	if _, ok := byRun["run-pending"]; !ok {
		t.Error("run-pending missing orphan_finalize flowlog")
	}
	if _, ok := byRun["run-running"]; !ok {
		t.Error("run-running missing orphan_finalize flowlog")
	}
	if _, ok := byRun["run-hitl"]; ok {
		t.Error("run-hitl (waiting_human) must NOT emit orphan_finalize")
	}
	// 归属字段抽查：run-running 的 pairs 必须带 team_id / target_status。
	foundTeam, foundTarget := false, false
	for _, p := range byRun["run-running"].pairs {
		if p.Key == "team_id" && p.Value == "team-1" {
			foundTeam = true
		}
		if p.Key == "target_status" && p.Value == TeamRunStatusFailed {
			foundTarget = true
		}
	}
	if !foundTeam || !foundTarget {
		t.Errorf("run-running pairs missing team_id/target_status: %+v", byRun["run-running"].pairs)
	}
	// session 归属：flowlog 挂在 run 的 session 上。
	if byRun["run-running"].sessionID != "sess-r" {
		t.Errorf("sessionID=%q, want sess-r", byRun["run-running"].sessionID)
	}
}

// flowLog 未注入（nil）时判死路径不 panic（向后兼容单测/离线工具）。
func TestRecoverOrphanedRunningTeamsEx_NilFlowLogSafe(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs:  []TeamRunRecord{{ID: "run-1", TeamID: "team-1", Status: TeamRunStatusRunning}},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})
	if _, err := uc.RecoverOrphanedRunningTeams(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := repo.casStatusFor("run-1"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-1: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
}

// --- Task 8（83 §4.2）：判死路径 graph_executions 终态收敛 ---

type stubGraphExecFinalizer struct {
	calls []struct {
		execID string
		failed bool
		errMsg string
	}
}

func (s *stubGraphExecFinalizer) FinalizeTeamGraphExecution(_ context.Context, execID string, failed bool, errMsg string) error {
	s.calls = append(s.calls, struct {
		execID string
		failed bool
		errMsg string
	}{execID, failed, errMsg})
	return nil
}

// 判死的 run 带 GraphExecutionID 时必须同步收敛 graph_executions（failed）；
// 无 GraphExecutionID 的 run 不调用；waiting_human 不触碰。
func TestRecoverOrphanedRunningTeamsEx_ConvergesGraphExecution(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs: []TeamRunRecord{
			{ID: "run-with-exec", TeamID: "team-1", SessionID: "sess-1", Status: TeamRunStatusRunning, GraphExecutionID: "exec-1"},
			{ID: "run-no-exec", TeamID: "team-1", SessionID: "sess-2", Status: TeamRunStatusPending},
			{ID: "run-hitl", TeamID: "team-1", SessionID: "sess-3", Status: TeamRunStatusWaitingHuman, GraphExecutionID: "exec-hitl"},
		},
	}
	finalizer := &stubGraphExecFinalizer{}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:             repo,
		Writer:             repo,
		RunReader:          repo,
		RunWriter:          repo,
		Lg:                 loggateway.NewNoop(),
		GraphExecFinalizer: finalizer,
	})

	if _, err := uc.RecoverOrphanedRunningTeams(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(finalizer.calls) != 1 {
		t.Fatalf("finalizer calls=%d, want 1 (only run-with-exec)", len(finalizer.calls))
	}
	call := finalizer.calls[0]
	if call.execID != "exec-1" {
		t.Errorf("execID=%q, want exec-1", call.execID)
	}
	if !call.failed {
		t.Error("finalizer must converge as failed=true")
	}
	if call.errMsg == "" {
		t.Error("errMsg must carry the finalize reason")
	}
	// 回归：run 状态转换不受影响。
	if s, ok := repo.casStatusFor("run-with-exec"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-with-exec: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
	if s, ok := repo.casStatusFor("run-hitl"); ok {
		t.Errorf("run-hitl must NOT be touched, got CAS to %q", s)
	}
}
