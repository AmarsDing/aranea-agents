package biz

import (
	"context"
	"testing"
)

type seedCronRepo struct {
	tasks map[string]CronTask
}

func newSeedCronRepo() *seedCronRepo {
	return &seedCronRepo{tasks: make(map[string]CronTask)}
}

func (m *seedCronRepo) ListCronTasks(_ context.Context) ([]CronTask, error) {
	out := make([]CronTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}

func (m *seedCronRepo) GetCronTask(_ context.Context, id string) (CronTask, error) {
	t, ok := m.tasks[id]
	if !ok {
		return CronTask{}, ErrNotFound
	}
	return t, nil
}

func (m *seedCronRepo) CreateCronTask(_ context.Context, t CronTask) (CronTask, error) {
	m.tasks[t.TaskKey] = t
	return t, nil
}

func (m *seedCronRepo) UpdateCronTask(_ context.Context, t CronTask) (CronTask, error) {
	m.tasks[t.TaskKey] = t
	return t, nil
}

func (m *seedCronRepo) DeleteCronTask(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func (m *seedCronRepo) GetCronTaskRun(_ context.Context, id string) (CronTaskRun, error) {
	return CronTaskRun{}, nil
}

func (m *seedCronRepo) ListCronTaskRuns(_ context.Context, _ CronTaskRunQuery) ([]CronTaskRun, int, error) {
	return nil, 0, nil
}

func (m *seedCronRepo) InsertCronTaskRun(_ context.Context, _ CronTaskRunInput) error {
	return nil
}

func (m *seedCronRepo) UpdateCronTaskRun(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func TestSeedModelRegistryCronTask_CreatesNew(t *testing.T) {
	repo := newSeedCronRepo()
	if err := SeedModelRegistryCronTask(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range repo.tasks {
		if task.TaskKey == "model-registry-sync" {
			found = true
			if !task.Enabled {
				t.Error("task should be enabled")
			}
			if task.Status != "active" {
				t.Errorf("expected active, got %s", task.Status)
			}
		}
	}
	if !found {
		t.Error("model-registry-sync task not created")
	}
}

func TestSeedModelRegistryCronTask_Idempotent(t *testing.T) {
	repo := newSeedCronRepo()
	if err := SeedModelRegistryCronTask(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if err := SeedModelRegistryCronTask(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, task := range repo.tasks {
		if task.TaskKey == "model-registry-sync" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 task after double seed, got %d", count)
	}
}
