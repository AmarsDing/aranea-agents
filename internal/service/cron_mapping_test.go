package service_test

import (
	"testing"

	v1 "aranea-agents/api/kratos/cron/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoCronTask_FullFields(t *testing.T) {
	task := biz.CronTask{
		ID:           "id-1",
		TaskKey:      "daily-report",
		Name:         "Daily Report",
		Description:  "Sends daily report",
		Status:       "active",
		Enabled:      true,
		SortOrder:    5,
		AgentID:      "agent-1",
		ConfigJSON:   `{"cron":"0 8 * * *"}`,
		MetadataJSON: `{"source":"import"}`,
		CreatedAt:    "2025-01-01T00:00:00Z",
		UpdatedAt:    "2025-01-02T00:00:00Z",
		DeletedAt:    "",
	}
	got := service.ToProtoCronTask(task)
	if got.Id != task.ID {
		t.Errorf("Id = %q, want %q", got.Id, task.ID)
	}
	if got.TaskKey != task.TaskKey {
		t.Errorf("TaskKey = %q, want %q", got.TaskKey, task.TaskKey)
	}
	if got.Name != task.Name {
		t.Errorf("Name = %q, want %q", got.Name, task.Name)
	}
	if got.Description != task.Description {
		t.Errorf("Description = %q, want %q", got.Description, task.Description)
	}
	if got.Status != task.Status {
		t.Errorf("Status = %q, want %q", got.Status, task.Status)
	}
	if got.Enabled != task.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, task.Enabled)
	}
	if got.SortOrder != int32(task.SortOrder) {
		t.Errorf("SortOrder = %d, want %d", got.SortOrder, task.SortOrder)
	}
	if got.AgentId != task.AgentID {
		t.Errorf("AgentId = %q, want %q", got.AgentId, task.AgentID)
	}
	if got.ConfigJson != task.ConfigJSON {
		t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, task.ConfigJSON)
	}
	if got.MetadataJson != task.MetadataJSON {
		t.Errorf("MetadataJson = %q, want %q", got.MetadataJson, task.MetadataJSON)
	}
	if got.CreatedAt != task.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, task.CreatedAt)
	}
	if got.UpdatedAt != task.UpdatedAt {
		t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, task.UpdatedAt)
	}
	if got.DeletedAt != task.DeletedAt {
		t.Errorf("DeletedAt = %q, want %q", got.DeletedAt, task.DeletedAt)
	}
}

func TestToProtoCronTask_ZeroValue(t *testing.T) {
	task := biz.CronTask{}
	got := service.ToProtoCronTask(task)
	if got.Id != "" {
		t.Errorf("Id = %q, want empty", got.Id)
	}
	if got.TaskKey != "" {
		t.Errorf("TaskKey = %q, want empty", got.TaskKey)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false")
	}
	if got.SortOrder != 0 {
		t.Errorf("SortOrder = %d, want 0", got.SortOrder)
	}
}

func TestToProtoCronTaskRun_FullFields(t *testing.T) {
	run := biz.CronTaskRun{
		ID:           "run-1",
		TaskID:       "task-1",
		TaskName:     "Daily Report",
		Status:       "completed",
		StartedAt:    "2025-01-01T08:00:00Z",
		FinishedAt:   "2025-01-01T08:05:00Z",
		Trigger:      "manual",
		RunID:        "exec-1",
		OutputJSON:   `{"rows":10}`,
		ErrorMessage: "",
		CreatedAt:    "2025-01-01T08:00:00Z",
	}
	got := service.ToProtoCronTaskRun(run)
	if got.Id != run.ID {
		t.Errorf("Id = %q, want %q", got.Id, run.ID)
	}
	if got.TaskId != run.TaskID {
		t.Errorf("TaskId = %q, want %q", got.TaskId, run.TaskID)
	}
	if got.TaskName != run.TaskName {
		t.Errorf("TaskName = %q, want %q", got.TaskName, run.TaskName)
	}
	if got.Status != run.Status {
		t.Errorf("Status = %q, want %q", got.Status, run.Status)
	}
	if got.StartedAt != run.StartedAt {
		t.Errorf("StartedAt = %q, want %q", got.StartedAt, run.StartedAt)
	}
	if got.FinishedAt != run.FinishedAt {
		t.Errorf("FinishedAt = %q, want %q", got.FinishedAt, run.FinishedAt)
	}
	if got.Trigger != run.Trigger {
		t.Errorf("Trigger = %q, want %q", got.Trigger, run.Trigger)
	}
	if got.RunId != run.RunID {
		t.Errorf("RunId = %q, want %q", got.RunId, run.RunID)
	}
	if got.OutputJson != run.OutputJSON {
		t.Errorf("OutputJson = %q, want %q", got.OutputJson, run.OutputJSON)
	}
	if got.ErrorMessage != run.ErrorMessage {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, run.ErrorMessage)
	}
	if got.CreatedAt != run.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, run.CreatedAt)
	}
}

func TestPatchFromProtoCronTask_FullFields(t *testing.T) {
	pb := &v1.CronTask{
		TaskKey:      "daily-report",
		Name:         "Daily Report",
		Description:  "Sends daily report",
		Status:       "active",
		Enabled:      true,
		SortOrder:    5,
		AgentId:      "agent-1",
		ConfigJson:   `{"cron":"0 8 * * *"}`,
		MetadataJson: `{"source":"import"}`,
	}
	got := service.PatchFromProtoCronTask(pb)
	if got.TaskKey == nil || *got.TaskKey != pb.TaskKey {
		t.Errorf("TaskKey = %v, want %q", got.TaskKey, pb.TaskKey)
	}
	if got.Name == nil || *got.Name != pb.Name {
		t.Errorf("Name = %v, want %q", got.Name, pb.Name)
	}
	if got.Description == nil || *got.Description != pb.Description {
		t.Errorf("Description = %v, want %q", got.Description, pb.Description)
	}
	if got.Status == nil || *got.Status != pb.Status {
		t.Errorf("Status = %v, want %q", got.Status, pb.Status)
	}
	if got.Enabled == nil || *got.Enabled != pb.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, pb.Enabled)
	}
	if got.SortOrder == nil || *got.SortOrder != int(pb.SortOrder) {
		t.Errorf("SortOrder = %v, want %d", got.SortOrder, pb.SortOrder)
	}
	if got.AgentID == nil || *got.AgentID != pb.AgentId {
		t.Errorf("AgentID = %v, want %q", got.AgentID, pb.AgentId)
	}
	if got.ConfigJSON == nil || *got.ConfigJSON != pb.ConfigJson {
		t.Errorf("ConfigJSON = %v, want %q", got.ConfigJSON, pb.ConfigJson)
	}
	if got.MetadataJSON == nil || *got.MetadataJSON != pb.MetadataJson {
		t.Errorf("MetadataJSON = %v, want %q", got.MetadataJSON, pb.MetadataJson)
	}
}

func TestPatchFromProtoCronTask_NilInput(t *testing.T) {
	got := service.PatchFromProtoCronTask(nil)
	if got.TaskKey != nil || got.Name != nil || got.Description != nil ||
		got.Status != nil || got.Enabled != nil || got.SortOrder != nil ||
		got.AgentID != nil || got.ConfigJSON != nil || got.MetadataJSON != nil {
		t.Error("expected all nil fields for nil input")
	}
}

func TestPatchFromProtoCronTask_PartialFields(t *testing.T) {
	pb := &v1.CronTask{
		Name:    "Updated Name",
		Enabled: false,
	}
	got := service.PatchFromProtoCronTask(pb)
	if got.TaskKey == nil || *got.TaskKey != "" {
		t.Errorf("TaskKey = %v, want pointer to empty string", got.TaskKey)
	}
	if got.Name == nil || *got.Name != "Updated Name" {
		t.Errorf("Name = %v, want pointer to 'Updated Name'", got.Name)
	}
	if got.Enabled == nil || *got.Enabled != false {
		t.Errorf("Enabled = %v, want pointer to false", got.Enabled)
	}
	if got.SortOrder == nil || *got.SortOrder != 0 {
		t.Errorf("SortOrder = %v, want pointer to 0", got.SortOrder)
	}
	if got.Description == nil || *got.Description != "" {
		t.Errorf("Description = %v, want pointer to empty string", got.Description)
	}
	if got.Status == nil || *got.Status != "" {
		t.Errorf("Status = %v, want pointer to empty string", got.Status)
	}
	if got.AgentID == nil || *got.AgentID != "" {
		t.Errorf("AgentID = %v, want pointer to empty string", got.AgentID)
	}
	if got.ConfigJSON == nil || *got.ConfigJSON != "" {
		t.Errorf("ConfigJSON = %v, want pointer to empty string", got.ConfigJSON)
	}
	if got.MetadataJSON == nil || *got.MetadataJSON != "" {
		t.Errorf("MetadataJSON = %v, want pointer to empty string", got.MetadataJSON)
	}
}
