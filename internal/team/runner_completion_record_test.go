package team

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// runnerCompletionMockRepo satisfies the 5 monitor repo interfaces used by
// biz.NewMonitorUsecase; only the runner.completion write path is recorded.
type runnerCompletionMockRepo struct {
	inserted []biz.MonitorEventWrite
	exists   bool
}

func (r *runnerCompletionMockRepo) ListAuditLogs(context.Context, biz.AuditQuery) (biz.AuditListResult, error) {
	return biz.AuditListResult{}, nil
}
func (r *runnerCompletionMockRepo) InsertAuditLog(context.Context, biz.AuditLog) error { return nil }
func (r *runnerCompletionMockRepo) DeleteAuditLogs(context.Context) (int, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) InsertMonitorEvent(_ context.Context, ev biz.MonitorEventWrite) error {
	r.inserted = append(r.inserted, ev)
	return nil
}
func (r *runnerCompletionMockRepo) ListMonitorEvents(context.Context, biz.MonitorEventsQuery) (biz.MonitorListResult, error) {
	return biz.MonitorListResult{}, nil
}
func (r *runnerCompletionMockRepo) GetMonitorEvent(context.Context, string) (biz.MonitorPlatformRow, error) {
	return biz.MonitorPlatformRow{}, nil
}
func (r *runnerCompletionMockRepo) CountMonitorEventsSince(context.Context, string, string, string, string) (int32, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) DeleteMonitorEventsOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) ListMonitorTraces(context.Context, biz.MonitorTracesQuery) (biz.MonitorListResult, error) {
	return biz.MonitorListResult{}, nil
}
func (r *runnerCompletionMockRepo) GetMonitorTrace(context.Context, string) (biz.MonitorPlatformRow, error) {
	return biz.MonitorPlatformRow{}, nil
}
func (r *runnerCompletionMockRepo) InsertMonitorTrace(context.Context, biz.MonitorTraceWrite) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpsertMonitorTraceSpan(context.Context, biz.MonitorTraceSpanWrite) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpdateMonitorTraceCompletion(context.Context, string, biz.MonitorTraceCompletion) error {
	return nil
}
func (r *runnerCompletionMockRepo) InterruptStaleTraces(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) EnsureTraceSchema(context.Context) error { return nil }
func (r *runnerCompletionMockRepo) ListAlertRules(context.Context) ([]biz.MonitorAlertRule, error) {
	return nil, nil
}
func (r *runnerCompletionMockRepo) ReplaceAlertRules(context.Context, []biz.MonitorAlertRule) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpdateAlertFiringState(context.Context, string, biz.MonitorAlertFiringState, *time.Time, float64, *time.Time) error {
	return nil
}
func (r *runnerCompletionMockRepo) ExistsRunnerCompletion(context.Context, string, string) (bool, error) {
	return r.exists, nil
}
func (r *runnerCompletionMockRepo) PatchRunnerCompletionMetadata(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (r *runnerCompletionMockRepo) AvgRunnerCompletionDurationMsSince(context.Context, string) (float64, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) LatencyPercentilesSince(context.Context, string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (r *runnerCompletionMockRepo) ListRecentRunnerCompletions(context.Context, time.Duration, int) ([]biz.RunnerCompletionRow, error) {
	return nil, nil
}

func newRunnerCompletionTestRunner(repo *runnerCompletionMockRepo, runRepo *gateRunRepo) *Runner {
	uc := biz.NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	return &Runner{
		runReader:       runRepo,
		runWriter:       runRepo,
		runTransitioner: &gateRunTransitioner{repo: runRepo},
		monitor:         uc,
		lg:              loggateway.NewNoop(),
	}
}

// F1 回归：team run 终态必须写 runner.completion 监控事件（Runner 指标 +
// runner.error_rate 告警的数据源）。经典团队与 spirit 编排团队都经
// team.Runner 执行，此处是统一漏斗。
func TestFinalizeTeamRun_RecordsRunnerCompletion(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := newRunnerCompletionTestRunner(repo, runRepo)

	sess, run, ar, asst := gateTestFixture()
	runRepo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1"}

	t0 := time.Now().Add(-2 * time.Second)
	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", t0, nil)

	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want success", got.Status)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("inserted=%d want 1", len(repo.inserted))
	}
	ev := repo.inserted[0]
	if ev.EventKey != "runner.completion" || ev.Status != "ok" {
		t.Fatalf("event=%+v", ev)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(ev.MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["run_kind"] != "team" || meta["team_id"] != "team-1" || meta["run_id"] != "run-1" || meta["session_id"] != "sess-1" {
		t.Fatalf("meta: %+v", meta)
	}
	if meta["duration_ms"].(float64) < 1000 {
		t.Fatalf("duration_ms=%v want >=1000 (t0 2s ago)", meta["duration_ms"])
	}
}

func TestFinishRunErr_RecordsRunnerCompletionError(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := newRunnerCompletionTestRunner(repo, runRepo)

	_, run, _, _ := gateTestFixture()
	runRepo.runs[run.ID] = run

	t0 := time.Now().Add(-1500 * time.Millisecond)
	runner.finishRunErr(context.Background(), &run, t0, "provider boom")

	if len(repo.inserted) != 1 {
		t.Fatalf("inserted=%d want 1", len(repo.inserted))
	}
	ev := repo.inserted[0]
	if ev.EventKey != "runner.completion" || ev.Status != "error" {
		t.Fatalf("event=%+v", ev)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(ev.MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	errObj, ok := meta["error"].(map[string]any)
	if !ok || errObj["message"] != "provider boom" {
		t.Fatalf("error meta: %+v", meta)
	}
}

// 已达终态的 run 不重复记录（finishRunErr 的 terminal 守卫）。
func TestFinishRunErr_TerminalRunSkipsCompletion(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := newRunnerCompletionTestRunner(repo, runRepo)

	_, run, _, _ := gateTestFixture()
	run.Status = biz.TeamRunStatusFailed
	runRepo.runs[run.ID] = run

	runner.finishRunErr(context.Background(), &run, time.Now(), "late error")

	if len(repo.inserted) != 0 {
		t.Fatalf("inserted=%d want 0 (terminal guard)", len(repo.inserted))
	}
}

// nil monitor 时不应 panic（测试/未接线路径）。
func TestFinalizeTeamRun_NilMonitorNoPanic(t *testing.T) {
	runRepo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	runner := newGateTestRunner(runRepo, event.NewV2Bus())

	sess, run, ar, asst := gateTestFixture()
	runRepo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want success", got.Status)
	}
}
