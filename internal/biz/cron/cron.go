// Package cron implements scheduled task management workflows.
package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync/atomic"

	"aranea-agents/pkg/apierror"
)

var cronIDRand uint64

func newCronTaskID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&cronIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// Task is one row of cron_task.
type Task struct {
	ID           string
	TaskKey      string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	AgentID      string
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

// TaskRun is one row of cron_task_run plus joined task display name and parsed output fields.
type TaskRun struct {
	ID           string
	TaskID       string
	TaskName     string
	Status       string
	StartedAt    string
	FinishedAt   string
	OutputJSON   string
	ErrorMessage string
	CreatedAt    string
	Trigger      string
	RunID        string
}

// TaskRunQuery filters ListCronTaskRuns.
type TaskRunQuery struct {
	TaskID string
	Status string
	Limit  int
}

// TaskRunInput is the insert payload for a cron task run.
type TaskRunInput struct {
	ID         string
	TaskID     string
	Status     string
	StartedAt  string
	OutputJSON string
	CreatedAt  string
}

// TaskPatch is a partial update for a cron task.
type TaskPatch struct {
	TaskKey      *string
	Name         *string
	Description  *string
	Status       *string
	Enabled      *bool
	SortOrder    *int
	AgentID      *string
	ConfigJSON   *string
	MetadataJSON *string
}

// StrPtr returns a pointer to the given string.
func StrPtr(s string) *string { return &s }

// BoolPtr returns a pointer to the given bool.
func BoolPtr(b bool) *bool { return &b }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// Repo abstracts cron task persistence.
type Repo interface {
	ListCronTasks(ctx context.Context) ([]Task, error)
	GetCronTask(ctx context.Context, id string) (Task, error)
	CreateCronTask(ctx context.Context, t Task) (Task, error)
	UpdateCronTask(ctx context.Context, t Task) (Task, error)
	DeleteCronTask(ctx context.Context, id string) error
	GetCronTaskRun(ctx context.Context, id string) (TaskRun, error)
	ListCronTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, error)
	InsertCronTaskRun(ctx context.Context, in TaskRunInput) error
	UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
}

// TaskTrigger enqueues immediate cron execution.
type TaskTrigger interface {
	TriggerTask(ctx context.Context, taskID string) (TaskRun, error)
}

// Usecase implements cron task CRUD and trigger workflows.
type Usecase struct {
	repo    Repo
	trigger TaskTrigger
}

// NewUsecase constructs a CronUsecase.
func NewUsecase(repo Repo, trigger TaskTrigger) *Usecase {
	return &Usecase{repo: repo, trigger: trigger}
}

// ListTasks returns all cron tasks.
func (u *Usecase) ListTasks(ctx context.Context) ([]Task, error) {
	return u.repo.ListCronTasks(ctx)
}

// GetTask returns one cron task by ID.
func (u *Usecase) GetTask(ctx context.Context, id string) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, apierror.BadRequest("CRON", "id is required")
	}
	return u.repo.GetCronTask(ctx, id)
}

// CreateTask validates and stores a new cron task.
func (u *Usecase) CreateTask(ctx context.Context, in Task) (Task, error) {
	in.TaskKey = strings.TrimSpace(in.TaskKey)
	in.Name = strings.TrimSpace(in.Name)
	if in.TaskKey == "" || in.Name == "" {
		return Task{}, apierror.BadRequest("CRON", "task_key and name are required")
	}
	if in.ID == "" {
		in.ID = newCronTaskID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := ValidateTaskConfig(in.ConfigJSON); err != nil {
		return Task{}, err
	}
	return u.repo.CreateCronTask(ctx, in)
}

// UpdateTask patches an existing cron task.
func (u *Usecase) UpdateTask(ctx context.Context, id string, patch TaskPatch) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, apierror.BadRequest("CRON", "id is required")
	}
	cur, err := u.repo.GetCronTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	merged := cur
	if patch.TaskKey != nil && *patch.TaskKey != "" {
		merged.TaskKey = *patch.TaskKey
	}
	if patch.Name != nil && *patch.Name != "" {
		merged.Name = *patch.Name
	}
	if patch.Status != nil && *patch.Status != "" {
		merged.Status = *patch.Status
	}
	// Description intentionally allows empty-string clearing (nil pointer =
	// skip, non-nil empty = clear). This is the documented behavior tested
	// by TestUsecase_UpdateTask/patch_description_allows_empty.
	if patch.Description != nil {
		merged.Description = *patch.Description
	}
	if patch.Enabled != nil {
		merged.Enabled = *patch.Enabled
	}
	if patch.SortOrder != nil {
		merged.SortOrder = *patch.SortOrder
	}
	// AgentID uses a zero-value guard (consistent with ConfigJSON/MetadataJSON)
	// to prevent proto3 zero-value clobbering when the caller only intends to
	// patch other fields (e.g. toggleRow sending only enabled/status). An empty
	// AgentID would leave the task orphaned with no executing agent.
	if patch.AgentID != nil && *patch.AgentID != "" {
		merged.AgentID = *patch.AgentID
	}
	// ConfigJSON and MetadataJSON contain structured data (cron expression, etc.)
	// and must not be overwritten by empty strings from proto3 zero values when
	// the caller only intends to patch other fields. A nil pointer still clears
	// the field via the explicit nil check; a non-nil empty string is ignored to
	// protect against proto3 zero-value clobbering.
	if patch.ConfigJSON != nil && *patch.ConfigJSON != "" {
		merged.ConfigJSON = *patch.ConfigJSON
	}
	if patch.MetadataJSON != nil && *patch.MetadataJSON != "" {
		merged.MetadataJSON = *patch.MetadataJSON
	}
	if merged.TaskKey == "" {
		merged.TaskKey = cur.TaskKey
	}
	if merged.Name == "" {
		merged.Name = cur.Name
	}
	if merged.Status == "" {
		merged.Status = cur.Status
	}
	if err := ValidateTaskConfig(merged.ConfigJSON); err != nil {
		return Task{}, err
	}
	return u.repo.UpdateCronTask(ctx, merged)
}

// DeleteTask removes a cron task.
func (u *Usecase) DeleteTask(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("CRON", "id is required")
	}
	return u.repo.DeleteCronTask(ctx, id)
}

// ListTaskRuns returns task run records.
func (u *Usecase) ListTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, error) {
	return u.repo.ListCronTaskRuns(ctx, q)
}

// GetTaskRun returns one task run by ID.
func (u *Usecase) GetTaskRun(ctx context.Context, id string) (TaskRun, error) {
	if strings.TrimSpace(id) == "" {
		return TaskRun{}, apierror.BadRequest("CRON", "run id is required")
	}
	return u.repo.GetCronTaskRun(ctx, id)
}

// TriggerTask enqueues an immediate execution.
func (u *Usecase) TriggerTask(ctx context.Context, id string) (TaskRun, error) {
	if u.trigger == nil {
		return TaskRun{}, ErrRunnerDisabled
	}
	if strings.TrimSpace(id) == "" {
		return TaskRun{}, apierror.BadRequest("CRON", "id is required")
	}
	return u.trigger.TriggerTask(ctx, id)
}

// ResetTaskFailures clears failure metadata and re-enables a task.
func (u *Usecase) ResetTaskFailures(ctx context.Context, id string) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, apierror.BadRequest("CRON", "id is required")
	}
	cur, err := u.repo.GetCronTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	metaJSON, err := ResetFailureMetadata(cur.MetadataJSON)
	if err != nil {
		return Task{}, apierror.BadRequest("CRON", "invalid metadata_json")
	}
	enabled := true
	status := "active"
	return u.UpdateTask(ctx, id, TaskPatch{
		Enabled:      &enabled,
		Status:       &status,
		MetadataJSON: &metaJSON,
	})
}

// ResetFailureMetadata clears failure-related fields in a cron task metadata JSON.
func ResetFailureMetadata(raw string) (string, error) {
	meta := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			return "", err
		}
	}
	meta["failure_count"] = 0
	meta["last_error"] = ""
	meta["recent_failures"] = []any{}
	out, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ValidateTaskConfig checks that config_json is valid JSON and contains required fields.
func ValidateTaskConfig(configJSON string) error {
	raw := strings.TrimSpace(configJSON)
	if raw == "" {
		return nil // empty config is allowed
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return apierror.BadRequest("CRON", "invalid config_json: "+err.Error())
	}
	// Validate target_type field if present
	if targetType, ok := cfg["target_type"]; ok {
		s, ok := targetType.(string)
		if !ok {
			return apierror.BadRequest("CRON", "config_json target_type must be a string")
		}
		switch s {
		case "agent", "team", "model_registry_sync":
			// valid
		default:
			return apierror.BadRequest("CRON", "config_json target_type must be one of: agent, team, model_registry_sync")
		}
	}
	// Validate cron_expression field if present
	if cronExpr, ok := cfg["cron_expression"]; ok {
		s, ok := cronExpr.(string)
		if !ok {
			return apierror.BadRequest("CRON", "config_json cron_expression must be a string")
		}
		if strings.TrimSpace(s) == "" {
			return apierror.BadRequest("CRON", "config_json cron_expression cannot be empty")
		}
	}
	return nil
}

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrRunnerDisabled = apierror.Unavailable("CRON", "cron runner disabled")
	ErrTaskDeleted    = apierror.NotFound("CRON", "cron task deleted")
	ErrSessionBusy    = apierror.Conflict("CRON", "cron session has active run")
	ErrNotFound       = apierror.NotFound("CRON", "cron task not found")
)
