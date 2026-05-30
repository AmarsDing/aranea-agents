package cron

import (
	"context"
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
)

type mockRepo struct {
	listFn    func(ctx context.Context) ([]Task, error)
	getFn     func(ctx context.Context, id string) (Task, error)
	createFn  func(ctx context.Context, t Task) (Task, error)
	updateFn  func(ctx context.Context, t Task) (Task, error)
	deleteFn  func(ctx context.Context, id string) error
	getRunFn  func(ctx context.Context, id string) (TaskRun, error)
	listRunFn func(ctx context.Context, q TaskRunQuery) ([]TaskRun, error)
	insertRunFn func(ctx context.Context, in TaskRunInput) error
	updateRunFn func(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
}

func (m *mockRepo) ListCronTasks(ctx context.Context) ([]Task, error) {
	return m.listFn(ctx)
}
func (m *mockRepo) GetCronTask(ctx context.Context, id string) (Task, error) {
	return m.getFn(ctx, id)
}
func (m *mockRepo) CreateCronTask(ctx context.Context, t Task) (Task, error) {
	return m.createFn(ctx, t)
}
func (m *mockRepo) UpdateCronTask(ctx context.Context, t Task) (Task, error) {
	return m.updateFn(ctx, t)
}
func (m *mockRepo) DeleteCronTask(ctx context.Context, id string) error {
	return m.deleteFn(ctx, id)
}
func (m *mockRepo) GetCronTaskRun(ctx context.Context, id string) (TaskRun, error) {
	return m.getRunFn(ctx, id)
}
func (m *mockRepo) ListCronTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, error) {
	return m.listRunFn(ctx, q)
}
func (m *mockRepo) InsertCronTaskRun(ctx context.Context, in TaskRunInput) error {
	return m.insertRunFn(ctx, in)
}
func (m *mockRepo) UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	return m.updateRunFn(ctx, id, status, finishedAt, outputJSON, errorMessage)
}

type mockTaskTrigger struct {
	triggerFn func(ctx context.Context, taskID string) (TaskRun, error)
}

func (m *mockTaskTrigger) TriggerTask(ctx context.Context, taskID string) (TaskRun, error) {
	return m.triggerFn(ctx, taskID)
}

func noOpRepo() *mockRepo {
	return &mockRepo{
		listFn:    func(_ context.Context) ([]Task, error) { return nil, nil },
		getFn:     func(_ context.Context, id string) (Task, error) { return Task{ID: id, TaskKey: "key", Name: "name", Status: "active"}, nil },
		createFn:  func(_ context.Context, t Task) (Task, error) { return t, nil },
		updateFn:  func(_ context.Context, t Task) (Task, error) { return t, nil },
		deleteFn:  func(_ context.Context, _ string) error { return nil },
		getRunFn:  func(_ context.Context, id string) (TaskRun, error) { return TaskRun{ID: id}, nil },
		listRunFn: func(_ context.Context, _ TaskRunQuery) ([]TaskRun, error) { return nil, nil },
		insertRunFn: func(_ context.Context, _ TaskRunInput) error { return nil },
		updateRunFn: func(_ context.Context, _, _, _, _, _ string) error { return nil },
	}
}

func noOpTrigger() *mockTaskTrigger {
	return &mockTaskTrigger{
		triggerFn: func(_ context.Context, taskID string) (TaskRun, error) {
			return TaskRun{TaskID: taskID}, nil
		},
	}
}

func TestUsecase_GetTask(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   bool
		errReason string
	}{
		{"empty id rejected", "", true, "CRON"},
		{"whitespace id rejected", "  ", true, "CRON"},
		{"valid id passes", "task-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUsecase(noOpRepo(), noOpTrigger())
			_, err := u.GetTask(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_CreateTask(t *testing.T) {
	tests := []struct {
		name      string
		in        Task
		repoFn    func(_ context.Context, t Task) (Task, error)
		wantErr   bool
		errReason string
		check     func(t *testing.T, got Task)
	}{
		{
			"empty task key rejected",
			Task{Name: "name"},
			nil, true, "CRON", nil,
		},
		{
			"whitespace task key rejected",
			Task{TaskKey: "  ", Name: "name"},
			nil, true, "CRON", nil,
		},
		{
			"empty name rejected",
			Task{TaskKey: "key"},
			nil, true, "CRON", nil,
		},
		{
			"whitespace name rejected",
			Task{TaskKey: "key", Name: "  "},
			nil, true, "CRON", nil,
		},
		{
			"defaults applied id and status",
			Task{TaskKey: "key", Name: "name"},
			func(_ context.Context, t Task) (Task, error) { return t, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.ID == "" {
					t.Error("ID should be auto-generated")
				}
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
			},
		},
		{
			"explicit id and status preserved",
			Task{TaskKey: "key", Name: "name", ID: "custom-id", Status: "paused"},
			func(_ context.Context, t Task) (Task, error) { return t, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.ID != "custom-id" {
					t.Errorf("ID = %q, want %q", got.ID, "custom-id")
				}
				if got.Status != "paused" {
					t.Errorf("Status = %q, want %q", got.Status, "paused")
				}
			},
		},
		{
			"whitespace trimmed",
			Task{TaskKey: "  key  ", Name: "  name  "},
			func(_ context.Context, t Task) (Task, error) { return t, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.TaskKey != "key" {
					t.Errorf("TaskKey = %q, want %q", got.TaskKey, "key")
				}
				if got.Name != "name" {
					t.Errorf("Name = %q, want %q", got.Name, "name")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.repoFn != nil {
				mr.createFn = tt.repoFn
			}
			u := NewUsecase(mr, noOpTrigger())
			got, err := u.CreateTask(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_UpdateTask(t *testing.T) {
	existingTask := Task{
		ID: "task-1", TaskKey: "original-key", Name: "Original",
		Description: "desc", Status: "active", Enabled: true,
		SortOrder: 1, AgentID: "agent-1",
		ConfigJSON:   `{"k":"v"}`,
		MetadataJSON: `{"m":1}`,
	}

	tests := []struct {
		name      string
		id        string
		patch     TaskPatch
		getFn     func(_ context.Context, id string) (Task, error)
		wantErr   bool
		errReason string
		check     func(t *testing.T, got Task)
	}{
		{
			"empty id rejected",
			"",
			TaskPatch{},
			nil, true, "CRON", nil,
		},
		{
			"whitespace id rejected",
			"  ",
			TaskPatch{},
			nil, true, "CRON", nil,
		},
		{
			"get task error",
			"task-1",
			TaskPatch{},
			func(_ context.Context, _ string) (Task, error) {
				return Task{}, errors.NotFound("CRON", "not found")
			},
			true, "CRON", nil,
		},
		{
			"patch task key",
			"task-1",
			TaskPatch{TaskKey: StrPtr("new-key")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.TaskKey != "new-key" {
					t.Errorf("TaskKey = %q, want %q", got.TaskKey, "new-key")
				}
			},
		},
		{
			"empty task key patch preserves original",
			"task-1",
			TaskPatch{TaskKey: StrPtr("")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.TaskKey != "original-key" {
					t.Errorf("TaskKey = %q, want %q", got.TaskKey, "original-key")
				}
			},
		},
		{
			"patch name",
			"task-1",
			TaskPatch{Name: StrPtr("New Name")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Name != "New Name" {
					t.Errorf("Name = %q, want %q", got.Name, "New Name")
				}
			},
		},
		{
			"empty name patch preserves original",
			"task-1",
			TaskPatch{Name: StrPtr("")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Name != "Original" {
					t.Errorf("Name = %q, want %q", got.Name, "Original")
				}
			},
		},
		{
			"patch status",
			"task-1",
			TaskPatch{Status: StrPtr("paused")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Status != "paused" {
					t.Errorf("Status = %q, want %q", got.Status, "paused")
				}
			},
		},
		{
			"empty status patch preserves original",
			"task-1",
			TaskPatch{Status: StrPtr("")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
			},
		},
		{
			"patch description allows empty",
			"task-1",
			TaskPatch{Description: StrPtr("")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Description != "" {
					t.Errorf("Description = %q, want empty", got.Description)
				}
			},
		},
		{
			"patch enabled",
			"task-1",
			TaskPatch{Enabled: BoolPtr(false)},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.Enabled {
					t.Error("Enabled should be false")
				}
			},
		},
		{
			"patch sort order",
			"task-1",
			TaskPatch{SortOrder: IntPtr(99)},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.SortOrder != 99 {
					t.Errorf("SortOrder = %d, want 99", got.SortOrder)
				}
			},
		},
		{
			"patch agent id",
			"task-1",
			TaskPatch{AgentID: StrPtr("agent-2")},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.AgentID != "agent-2" {
					t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-2")
				}
			},
		},
		{
			"patch config json",
			"task-1",
			TaskPatch{ConfigJSON: StrPtr(`{"new":"config"}`)},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.ConfigJSON != `{"new":"config"}` {
					t.Errorf("ConfigJSON = %q, want updated", got.ConfigJSON)
				}
			},
		},
		{
			"patch metadata json",
			"task-1",
			TaskPatch{MetadataJSON: StrPtr(`{"reset":true}`)},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.MetadataJSON != `{"reset":true}` {
					t.Errorf("MetadataJSON = %q, want updated", got.MetadataJSON)
				}
			},
		},
		{
			"nil patch fields preserve all originals",
			"task-1",
			TaskPatch{},
			func(_ context.Context, id string) (Task, error) { return existingTask, nil },
			false, "",
			func(t *testing.T, got Task) {
				if got.TaskKey != existingTask.TaskKey {
					t.Errorf("TaskKey = %q, want %q", got.TaskKey, existingTask.TaskKey)
				}
				if got.Name != existingTask.Name {
					t.Errorf("Name = %q, want %q", got.Name, existingTask.Name)
				}
				if got.Status != existingTask.Status {
					t.Errorf("Status = %q, want %q", got.Status, existingTask.Status)
				}
				if got.Description != existingTask.Description {
					t.Errorf("Description = %q, want %q", got.Description, existingTask.Description)
				}
				if got.Enabled != existingTask.Enabled {
					t.Errorf("Enabled = %v, want %v", got.Enabled, existingTask.Enabled)
				}
				if got.SortOrder != existingTask.SortOrder {
					t.Errorf("SortOrder = %d, want %d", got.SortOrder, existingTask.SortOrder)
				}
				if got.AgentID != existingTask.AgentID {
					t.Errorf("AgentID = %q, want %q", got.AgentID, existingTask.AgentID)
				}
				if got.ConfigJSON != existingTask.ConfigJSON {
					t.Errorf("ConfigJSON = %q, want %q", got.ConfigJSON, existingTask.ConfigJSON)
				}
				if got.MetadataJSON != existingTask.MetadataJSON {
					t.Errorf("MetadataJSON = %q, want %q", got.MetadataJSON, existingTask.MetadataJSON)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.getFn != nil {
				mr.getFn = tt.getFn
			}
			u := NewUsecase(mr, noOpTrigger())
			got, err := u.UpdateTask(context.Background(), tt.id, tt.patch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_DeleteTask(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   bool
		errReason string
	}{
		{"empty id rejected", "", true, "CRON"},
		{"whitespace id rejected", "  ", true, "CRON"},
		{"valid id passes", "task-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUsecase(noOpRepo(), noOpTrigger())
			err := u.DeleteTask(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_TriggerTask(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		trigger   TaskTrigger
		wantErr   bool
		errReason string
	}{
		{"nil trigger returns ErrRunnerDisabled", "task-1", nil, true, ""},
		{"empty id rejected", "", noOpTrigger(), true, "CRON"},
		{"whitespace id rejected", "  ", noOpTrigger(), true, "CRON"},
		{"valid id with trigger", "task-1", noOpTrigger(), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUsecase(noOpRepo(), tt.trigger)
			_, err := u.TriggerTask(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.name == "nil trigger returns ErrRunnerDisabled" {
				if err.Error() != "cron runner disabled" {
					t.Errorf("err = %q, want ErrRunnerDisabled", err.Error())
				}
			}
		})
	}
}

func TestUsecase_ResetTaskFailures(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		getFn     func(_ context.Context, id string) (Task, error)
		wantErr   bool
		errReason string
		check     func(t *testing.T, got Task)
	}{
		{
			"empty id rejected",
			"",
			nil, true, "CRON", nil,
		},
		{
			"whitespace id rejected",
			"  ",
			nil, true, "CRON", nil,
		},
		{
			"get task error",
			"task-1",
			func(_ context.Context, _ string) (Task, error) {
				return Task{}, errors.NotFound("CRON", "not found")
			},
			true, "CRON", nil,
		},
		{
			"invalid metadata json",
			"task-1",
			func(_ context.Context, _ string) (Task, error) {
				return Task{ID: "task-1", TaskKey: "key", Name: "name", Status: "disabled", MetadataJSON: "not-json"}, nil
			},
			true, "CRON", nil,
		},
		{
			"resets failures and re-enables",
			"task-1",
			func(_ context.Context, _ string) (Task, error) {
				return Task{
					ID: "task-1", TaskKey: "key", Name: "name",
					Status: "disabled", Enabled: false,
					MetadataJSON: `{"failure_count":5,"last_error":"timeout","recent_failures":["e1"]}`,
				}, nil
			},
			false, "",
			func(t *testing.T, got Task) {
				if !got.Enabled {
					t.Error("Enabled should be true after reset")
				}
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
			},
		},
		{
			"empty metadata still succeeds",
			"task-1",
			func(_ context.Context, _ string) (Task, error) {
				return Task{
					ID: "task-1", TaskKey: "key", Name: "name",
					Status: "disabled", Enabled: false, MetadataJSON: "",
				}, nil
			},
			false, "",
			func(t *testing.T, got Task) {
				if !got.Enabled {
					t.Error("Enabled should be true after reset")
				}
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpRepo()
			if tt.getFn != nil {
				mr.getFn = tt.getFn
			}
			u := NewUsecase(mr, noOpTrigger())
			got, err := u.ResetTaskFailures(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
