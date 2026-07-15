package service_test

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/cron/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// memCronRepo is an in-memory CronRepo.
type memCronRepo struct {
	tasks map[string]biz.CronTask
	runs  []biz.CronTaskRun
}

func newMemCronRepo() *memCronRepo {
	return &memCronRepo{tasks: make(map[string]biz.CronTask)}
}

func (m *memCronRepo) ListCronTasks(_ context.Context) ([]biz.CronTask, error) {
	out := make([]biz.CronTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}

func (m *memCronRepo) GetCronTask(_ context.Context, id string) (biz.CronTask, error) {
	t, ok := m.tasks[id]
	if !ok {
		return biz.CronTask{}, fmt.Errorf("cron task not found: %s", id)
	}
	return t, nil
}

func (m *memCronRepo) CreateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}

func (m *memCronRepo) UpdateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}

func (m *memCronRepo) DeleteCronTask(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func (m *memCronRepo) ListCronTaskRuns(_ context.Context, _ biz.CronTaskRunQuery) ([]biz.CronTaskRun, error) {
	return m.runs, nil
}

func (m *memCronRepo) InsertCronTaskRun(_ context.Context, in biz.CronTaskRunInput) error {
	m.runs = append(m.runs, biz.CronTaskRun{ID: in.ID, TaskID: in.TaskID, Status: in.Status})
	return nil
}

func (m *memCronRepo) UpdateCronTaskRun(_ context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	return nil
}

func newCronService() *service.CronService {
	return service.NewCronService(biz.NewCronUsecase(newMemCronRepo(), nil))
}

func (m *memCronRepo) GetCronTaskRun(_ context.Context, id string) (biz.CronTaskRun, error) {
	for _, run := range m.runs {
		if run.ID == id {
			return run, nil
		}
	}
	return biz.CronTaskRun{}, fmt.Errorf("cron run not found: %s", id)
}

func TestCronService_CreateListGetDelete(t *testing.T) {
	svc := newCronService()
	ctx := context.Background()

	created, err := svc.CreateCronTask(ctx, &v1.CreateCronTaskRequest{
		TaskKey: "daily-report",
		Name:    "Daily Report",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetTaskKey() != "daily-report" {
		t.Errorf("task_key mismatch: %s", created.GetTaskKey())
	}

	list, err := svc.ListCronTasks(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.GetItems()) != 1 {
		t.Errorf("expected 1, got %d", len(list.GetItems()))
	}

	got, err := svc.GetCronTask(ctx, &v1.GetCronTaskRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetId() != created.GetId() {
		t.Errorf("id mismatch")
	}

	_, err = svc.DeleteCronTask(ctx, &v1.DeleteCronTaskRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	list2, _ := svc.ListCronTasks(ctx, &emptypb.Empty{})
	if len(list2.GetItems()) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list2.GetItems()))
	}
}

func TestCronService_Create_Validation(t *testing.T) {
	svc := newCronService()
	ctx := context.Background()

	_, err := svc.CreateCronTask(ctx, &v1.CreateCronTaskRequest{Name: "no-key"})
	if err == nil {
		t.Error("expected error for missing task_key")
	}

	_, err = svc.CreateCronTask(ctx, &v1.CreateCronTaskRequest{TaskKey: "no-name"})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestCronService_Update_RequiresBody(t *testing.T) {
	svc := newCronService()
	ctx := context.Background()
	_, err := svc.UpdateCronTask(ctx, &v1.UpdateCronTaskRequest{Id: "any"})
	if err == nil {
		t.Error("expected error for nil task body")
	}
}

func TestCronService_ListTaskRuns(t *testing.T) {
	svc := newCronService()
	ctx := context.Background()

	created, err := svc.CreateCronTask(ctx, &v1.CreateCronTaskRequest{
		TaskKey: "list-runs", Name: "List Runs",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resp, err := svc.ListCronTaskRuns(ctx, &v1.ListCronTaskRunsRequest{CronTaskId: created.GetId()})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}
