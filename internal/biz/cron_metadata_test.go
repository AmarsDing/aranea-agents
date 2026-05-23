package biz

import (
	"context"
	"encoding/json"
	"testing"
)

func TestResetCronFailureMetadata(t *testing.T) {
	raw, err := ResetCronFailureMetadata(`{"failure_count":3,"last_error":"boom","recent_failures":[{"started_at":"t","error_message":"e"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["failure_count"].(float64) != 0 {
		t.Fatalf("expected failure_count 0, got %v", meta["failure_count"])
	}
	if meta["last_error"] != "" {
		t.Fatalf("expected empty last_error")
	}
}

func TestCronUsecase_ResetTaskFailures(t *testing.T) {
	repo := &memCronRepoReset{tasks: map[string]CronTask{}}
	uc := NewCronUsecase(repo, nil)
	ctx := context.Background()
	created, err := uc.CreateTask(ctx, CronTask{
		ID:           "t1",
		TaskKey:      "job",
		Name:         "Job",
		Status:       "dead",
		Enabled:      false,
		MetadataJSON: `{"failure_count":3,"last_error":"x"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := uc.ResetTaskFailures(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.Status != "active" {
		t.Fatalf("expected active enabled task, got enabled=%v status=%s", out.Enabled, out.Status)
	}
	var meta map[string]any
	_ = json.Unmarshal([]byte(out.MetadataJSON), &meta)
	if meta["failure_count"].(float64) != 0 {
		t.Fatalf("expected reset failure_count")
	}
}

type memCronRepoReset struct {
	tasks map[string]CronTask
}

func (m *memCronRepoReset) ListCronTasks(context.Context) ([]CronTask, error) { return nil, nil }
func (m *memCronRepoReset) GetCronTask(_ context.Context, id string) (CronTask, error) {
	return m.tasks[id], nil
}
func (m *memCronRepoReset) CreateCronTask(_ context.Context, t CronTask) (CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}
func (m *memCronRepoReset) UpdateCronTask(_ context.Context, t CronTask) (CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}
func (m *memCronRepoReset) DeleteCronTask(context.Context, string) error { return nil }
func (m *memCronRepoReset) GetCronTaskRun(context.Context, string) (CronTaskRun, error) {
	return CronTaskRun{}, nil
}
func (m *memCronRepoReset) ListCronTaskRuns(context.Context, CronTaskRunQuery) ([]CronTaskRun, error) {
	return nil, nil
}
func (m *memCronRepoReset) InsertCronTaskRun(context.Context, CronTaskRunInput) error { return nil }
func (m *memCronRepoReset) UpdateCronTaskRun(context.Context, string, string, string, string, string) error {
	return nil
}
