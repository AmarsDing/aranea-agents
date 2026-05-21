package cronrunner

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type memCronExecRepo struct {
	mu    sync.Mutex
	tasks map[string]biz.CronTask
	runs  map[string]biz.CronTaskRun
}

func newMemCronExecRepo() *memCronExecRepo {
	return &memCronExecRepo{
		tasks: map[string]biz.CronTask{
			"t1": {
				ID:           "t1",
				TaskKey:      "job",
				Name:         "Job",
				Status:       "active",
				Enabled:      true,
				ConfigJSON:   `{"message":"hi","schedule_type":"interval","interval_seconds":60}`,
				MetadataJSON: `{"run_count":2,"failure_count":1,"success_count":1}`,
			},
		},
		runs: map[string]biz.CronTaskRun{},
	}
}

func (m *memCronExecRepo) ListCronTasks(context.Context) ([]biz.CronTask, error) { return nil, nil }
func (m *memCronExecRepo) GetCronTask(_ context.Context, id string) (biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return biz.CronTask{}, biz.ErrCronTaskDeleted
	}
	return t, nil
}
func (m *memCronExecRepo) CreateCronTask(context.Context, biz.CronTask) (biz.CronTask, error) {
	return biz.CronTask{}, nil
}
func (m *memCronExecRepo) UpdateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[t.ID] = t
	return t, nil
}
func (m *memCronExecRepo) DeleteCronTask(context.Context, string) error { return nil }
func (m *memCronExecRepo) GetCronTaskRun(_ context.Context, id string) (biz.CronTaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id], nil
}
func (m *memCronExecRepo) ListCronTaskRuns(context.Context, biz.CronTaskRunQuery) ([]biz.CronTaskRun, error) {
	return nil, nil
}
func (m *memCronExecRepo) InsertCronTaskRun(_ context.Context, in biz.CronTaskRunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[in.ID] = biz.CronTaskRun{ID: in.ID, TaskID: in.TaskID, Status: in.Status, StartedAt: in.StartedAt}
	return nil
}
func (m *memCronExecRepo) UpdateCronTaskRun(_ context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.runs[id]
	run.Status = status
	run.FinishedAt = finishedAt
	run.OutputJSON = outputJSON
	run.ErrorMessage = errorMessage
	m.runs[id] = run
	return nil
}

func TestFinishTaskRun_ReloadsMetadataBeforeFinalize(t *testing.T) {
	repo := newMemCronExecRepo()
	r := &Runner{deps: Deps{Cron: repo}}
	cfg, err := parseCronTaskConfig(repo.tasks["t1"].ConfigJSON)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate another writer bumping run_count while dispatch runs.
	repo.tasks["t1"] = biz.CronTask{
		ID:           "t1",
		TaskKey:      "job",
		Name:         "Job",
		Status:       "active",
		Enabled:      true,
		ConfigJSON:   repo.tasks["t1"].ConfigJSON,
		MetadataJSON: `{"run_count":9,"failure_count":1,"success_count":8}`,
	}

	r.finishTaskRun(context.Background(), "t1", "run-1", "2026-01-01T00:00:00Z", "schedule", cfg, runOutcome{status: "success"})

	var meta map[string]any
	if err := json.Unmarshal([]byte(repo.tasks["t1"].MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["run_count"].(float64) != 10 {
		t.Fatalf("expected run_count=10 after reload+increment, got %v", meta["run_count"])
	}
	if meta["failure_count"].(float64) != 0 {
		t.Fatalf("expected schedule success to reset failure_count, got %v", meta["failure_count"])
	}
}

func TestFinalizeRun_SkippedDoesNotIncrementFailureCount(t *testing.T) {
	repo := newMemCronExecRepo()
	r := &Runner{deps: Deps{Cron: repo}}
	cfg, _ := parseCronTaskConfig(repo.tasks["t1"].ConfigJSON)
	meta := parseCronTaskMetadata(repo.tasks["t1"].MetadataJSON)

	r.finalizeRun(context.Background(), "run-skipped", repo.tasks["t1"], cfg, meta, "2026-01-01T00:00:00Z", "schedule", runOutcome{
		status: "skipped",
		errMsg: biz.ErrCronSessionBusy.Error(),
	})

	var out map[string]any
	_ = json.Unmarshal([]byte(repo.tasks["t1"].MetadataJSON), &out)
	if out["failure_count"].(float64) != 1 {
		t.Fatalf("skipped should not increment failure_count, got %v", out["failure_count"])
	}
	if out["last_run_status"] != "skipped" {
		t.Fatalf("expected last_run_status=skipped, got %v", out["last_run_status"])
	}
	if repo.runs["run-skipped"].Status != "skipped" {
		t.Fatalf("expected run status skipped, got %s", repo.runs["run-skipped"].Status)
	}
}

func TestRecordScheduleFailure_CreatesRunAndIncrementsFailure(t *testing.T) {
	repo := newMemCronExecRepo()
	r := &Runner{deps: Deps{Cron: repo}}
	now := time.Now().UTC()
	cfg, _ := parseCronTaskConfig(`{"message":"","schedule_type":"interval","interval_seconds":60}`)

	r.recordScheduleFailure(context.Background(), repo.tasks["t1"], cfg, now, "cron message is required")

	if len(repo.runs) != 1 {
		t.Fatalf("expected 1 cron_task_run, got %d", len(repo.runs))
	}
	var run biz.CronTaskRun
	for _, item := range repo.runs {
		run = item
	}
	if run.Status != "failure" {
		t.Fatalf("expected run status failure, got %s", run.Status)
	}
	var meta map[string]any
	_ = json.Unmarshal([]byte(repo.tasks["t1"].MetadataJSON), &meta)
	if meta["failure_count"].(float64) != 2 {
		t.Fatalf("expected failure_count incremented to 2, got %v", meta["failure_count"])
	}
	if meta["last_run_status"] != "failure" {
		t.Fatalf("expected last_run_status=failure, got %v", meta["last_run_status"])
	}
}

func TestFinalizeRun_ManualFailureDoesNotIncrementDeadCounter(t *testing.T) {
	repo := newMemCronExecRepo()
	repo.tasks["t1"] = biz.CronTask{
		ID:           "t1",
		TaskKey:      "job",
		Name:         "Job",
		Status:       "active",
		Enabled:      true,
		ConfigJSON:   `{"message":"hi","schedule_type":"interval","interval_seconds":60}`,
		MetadataJSON: `{"failure_count":2}`,
	}
	r := &Runner{deps: Deps{Cron: repo}}
	cfg, _ := parseCronTaskConfig(repo.tasks["t1"].ConfigJSON)
	meta := parseCronTaskMetadata(repo.tasks["t1"].MetadataJSON)

	r.finalizeRun(context.Background(), "run-1", repo.tasks["t1"], cfg, meta, "2026-01-01T00:00:00Z", "manual", runOutcome{
		status: "failure",
		errMsg: "boom",
	})

	var out map[string]any
	_ = json.Unmarshal([]byte(repo.tasks["t1"].MetadataJSON), &out)
	if out["failure_count"].(float64) != 2 {
		t.Fatalf("manual failure should not bump failure_count for dead letter, got %v", out["failure_count"])
	}
	if repo.tasks["t1"].Status == "dead" {
		t.Fatal("manual failure must not mark task dead")
	}
}
