package cron

import (
	"context"
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
)

func TestNewCronTaskID_NonEmpty(t *testing.T) {
	id := newCronTaskID()
	if len(id) != 24 {
		t.Errorf("newCronTaskID() length = %d, want 24", len(id))
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("newCronTaskID() contains non-hex char %q", c)
			break
		}
	}
}

func TestErrRunnerDisabled(t *testing.T) {
	if ErrRunnerDisabled.Error() != "cron runner disabled" {
		t.Errorf("ErrRunnerDisabled.Error() = %q, want %q", ErrRunnerDisabled.Error(), "cron runner disabled")
	}
}

func TestErrTaskDeleted(t *testing.T) {
	if ErrTaskDeleted.Error() != "cron task deleted" {
		t.Errorf("ErrTaskDeleted.Error() = %q, want %q", ErrTaskDeleted.Error(), "cron task deleted")
	}
}

func TestErrSessionBusy(t *testing.T) {
	if ErrSessionBusy.Error() != "cron session has active run" {
		t.Errorf("ErrSessionBusy.Error() = %q, want %q", ErrSessionBusy.Error(), "cron session has active run")
	}
}

func TestUsecase_GetTask_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.GetTask(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_GetTask_Valid(t *testing.T) {
	want := Task{ID: "abc", TaskKey: "tk", Name: "n"}
	repo := noOpRepo()
	repo.getFn = func(_ context.Context, id string) (Task, error) {
		if id != "abc" {
			t.Errorf("id = %q, want %q", id, "abc")
		}
		return want, nil
	}
	u := NewUsecase(repo, noOpTrigger())
	got, err := u.GetTask(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("got.ID = %q, want %q", got.ID, want.ID)
	}
}

func TestUsecase_CreateTask_EmptyKey(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.CreateTask(context.Background(), Task{TaskKey: "", Name: "n"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_CreateTask_EmptyName(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.CreateTask(context.Background(), Task{TaskKey: "tk", Name: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_CreateTask_GeneratesID(t *testing.T) {
	var captured Task
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, t Task) (Task, error) {
		captured = t
		return t, nil
	}
	u := NewUsecase(repo, noOpTrigger())
	_, err := u.CreateTask(context.Background(), Task{TaskKey: "tk", Name: "n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ID == "" {
		t.Error("expected non-empty generated ID")
	}
	if len(captured.ID) != 24 {
		t.Errorf("generated ID length = %d, want 24", len(captured.ID))
	}
}

func TestUsecase_CreateTask_DefaultStatus(t *testing.T) {
	var captured Task
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, t Task) (Task, error) {
		captured = t
		return t, nil
	}
	u := NewUsecase(repo, noOpTrigger())
	_, err := u.CreateTask(context.Background(), Task{TaskKey: "tk", Name: "n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Status != "active" {
		t.Errorf("Status = %q, want %q", captured.Status, "active")
	}
}

func TestUsecase_CreateTask_ExistingID(t *testing.T) {
	var captured Task
	repo := noOpRepo()
	repo.createFn = func(_ context.Context, t Task) (Task, error) {
		captured = t
		return t, nil
	}
	u := NewUsecase(repo, noOpTrigger())
	_, err := u.CreateTask(context.Background(), Task{ID: "my-id", TaskKey: "tk", Name: "n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ID != "my-id" {
		t.Errorf("ID = %q, want %q", captured.ID, "my-id")
	}
}

func TestUsecase_UpdateTask_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.UpdateTask(context.Background(), "", TaskPatch{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_UpdateTask_MergePatch(t *testing.T) {
	existing := Task{
		ID: "x", TaskKey: "old-key", Name: "old-name", Status: "active",
		Description: "old-desc", Enabled: false, SortOrder: 1,
		AgentID: "a1", ConfigJSON: "{}", MetadataJSON: "{}",
	}
	newKey := "new-key"
	newName := "new-name"
	newStatus := "paused"
	newDesc := "new-desc"
	newEnabled := true
	newSort := 5
	newAgent := "a2"
	newConfig := `{"k":"v"}`
	newMeta := `{"m":1}`
	var captured Task
	repo := noOpRepo()
	repo.getFn = func(_ context.Context, id string) (Task, error) {
		return existing, nil
	}
	repo.updateFn = func(_ context.Context, t Task) (Task, error) {
		captured = t
		return t, nil
	}
	u := NewUsecase(repo, noOpTrigger())
	_, err := u.UpdateTask(context.Background(), "x", TaskPatch{
		TaskKey: &newKey, Name: &newName, Status: &newStatus,
		Description: &newDesc, Enabled: &newEnabled, SortOrder: &newSort,
		AgentID: &newAgent, ConfigJSON: &newConfig, MetadataJSON: &newMeta,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.TaskKey != newKey {
		t.Errorf("TaskKey = %q, want %q", captured.TaskKey, newKey)
	}
	if captured.Name != newName {
		t.Errorf("Name = %q, want %q", captured.Name, newName)
	}
	if captured.Status != newStatus {
		t.Errorf("Status = %q, want %q", captured.Status, newStatus)
	}
	if captured.Description != newDesc {
		t.Errorf("Description = %q, want %q", captured.Description, newDesc)
	}
	if captured.Enabled != newEnabled {
		t.Errorf("Enabled = %v, want %v", captured.Enabled, newEnabled)
	}
	if captured.SortOrder != newSort {
		t.Errorf("SortOrder = %d, want %d", captured.SortOrder, newSort)
	}
	if captured.AgentID != newAgent {
		t.Errorf("AgentID = %q, want %q", captured.AgentID, newAgent)
	}
	if captured.ConfigJSON != newConfig {
		t.Errorf("ConfigJSON = %q, want %q", captured.ConfigJSON, newConfig)
	}
	if captured.MetadataJSON != newMeta {
		t.Errorf("MetadataJSON = %q, want %q", captured.MetadataJSON, newMeta)
	}
}

func TestUsecase_DeleteTask_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	err := u.DeleteTask(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_GetTaskRun_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.GetTaskRun(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_TriggerTask_NilTrigger(t *testing.T) {
	u := NewUsecase(noOpRepo(), nil)
	_, err := u.TriggerTask(context.Background(), "some-id")
	if err != ErrRunnerDisabled {
		t.Errorf("err = %v, want ErrRunnerDisabled", err)
	}
}

func TestUsecase_TriggerTask_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.TriggerTask(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestUsecase_ResetTaskFailures_EmptyID(t *testing.T) {
	u := NewUsecase(noOpRepo(), noOpTrigger())
	_, err := u.ResetTaskFailures(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}
