// Package cron implements scheduled task management workflows.
package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	// WorkspaceID is the tenant isolation scope.
	// empty = legacy/system-owned (visible to system caller only);
	// non-empty = tenant-private (visible only to owning workspace).
	WorkspaceID string
	CreatedAt   string
	UpdatedAt   string
	DeletedAt   string
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
	// ResolvedAt 非空 = 该次失败已被人工忽略/核销（workbench 告警 lane）。
	ResolvedAt string
	ResolvedBy string
}

// TaskRunQuery filters ListCronTaskRuns.
type TaskRunQuery struct {
	TaskID string
	Status string
	Limit  int
	Offset int
	// IncludeResolved 为 true 时连已核销失败一起返回；默认只返回未核销。
	IncludeResolved bool
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
	ListCronTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, int, error)
	InsertCronTaskRun(ctx context.Context, in TaskRunInput) error
	UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
	// ResolveCronTaskFailures 把该运行所属任务的全部未核销失败标记为已核销，
	// 返回被点击运行的刷新后记录与本次核销条数。
	ResolveCronTaskFailures(ctx context.Context, runID, actor string) (TaskRun, int, error)
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
	// 调度变了、或从停用改回启用：旧的 next_run_at 已不代表新计划（否则会按旧时间再跑一次，
	// 或一启用就补跑停用期间错过的那次），清掉让 runner 下一个 tick 按新计划重算。
	if scheduleChanged(cur.ConfigJSON, merged.ConfigJSON) || (!cur.Enabled && merged.Enabled) {
		merged.MetadataJSON = clearNextRunAt(merged.MetadataJSON)
	}
	return u.repo.UpdateCronTask(ctx, merged)
}

// scheduleKeys 是 config_json 里决定「什么时候跑」的字段。
var scheduleKeys = []string{"schedule_type", "cron_expression", "interval_seconds", "run_at", "timezone"}

func scheduleChanged(before, after string) bool {
	a, b := map[string]any{}, map[string]any{}
	_ = json.Unmarshal([]byte(defaultObject(before)), &a)
	_ = json.Unmarshal([]byte(defaultObject(after)), &b)
	for _, k := range scheduleKeys {
		if fmt.Sprint(a[k]) != fmt.Sprint(b[k]) {
			return true
		}
	}
	return false
}

// clearNextRunAt 去掉 metadata 里的 next_run_at；解析失败原样返回（不因为清缓存丢统计）。
func clearNextRunAt(raw string) string {
	meta := map[string]any{}
	if err := json.Unmarshal([]byte(defaultObject(raw)), &meta); err != nil {
		return raw
	}
	if _, ok := meta["next_run_at"]; !ok {
		return raw
	}
	delete(meta, "next_run_at")
	out, err := json.Marshal(meta)
	if err != nil {
		return raw
	}
	return string(out)
}

func defaultObject(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

// DeleteTask removes a cron task.
func (u *Usecase) DeleteTask(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("CRON", "id is required")
	}
	return u.repo.DeleteCronTask(ctx, id)
}

// ListTaskRuns returns task run records plus the total count under the same filters.
func (u *Usecase) ListTaskRuns(ctx context.Context, q TaskRunQuery) ([]TaskRun, int, error) {
	return u.repo.ListCronTaskRuns(ctx, q)
}

// GetTaskRun returns one task run by ID.
func (u *Usecase) GetTaskRun(ctx context.Context, id string) (TaskRun, error) {
	if strings.TrimSpace(id) == "" {
		return TaskRun{}, apierror.BadRequest("CRON", "run id is required")
	}
	return u.repo.GetCronTaskRun(ctx, id)
}

// ResolveTaskRun 核销一次失败（同任务全部未核销失败一并核销），返回刷新后运行与条数。
func (u *Usecase) ResolveTaskRun(ctx context.Context, id, actor string) (TaskRun, int, error) {
	if strings.TrimSpace(id) == "" {
		return TaskRun{}, 0, apierror.BadRequest("CRON", "run id is required")
	}
	return u.repo.ResolveCronTaskFailures(ctx, id, actor)
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
		case "agent", "team", "model_registry_sync", "mvp_rss_ingest", "gm_patrol", "gm_report":
			// valid — mvp_rss_ingest pulls RSS into Content Inbox (ops path);
			// gm_patrol / gm_report 是看板总经理 Agent 的巡视与汇报（03-看板 §14）
		default:
			return apierror.BadRequest("CRON", "config_json target_type must be one of: agent, team, model_registry_sync, mvp_rss_ingest, gm_patrol, gm_report")
		}
	}
	// cron_expression 只对 schedule_type=cron 有意义；其余类型（含缺省的 interval）带空串是表单常态，
	// 此前一律拒绝空串，导致界面上「每隔 / 一次」类任务根本存不进去。
	expr, hasExpr := cfg["cron_expression"]
	exprStr, isStr := expr.(string)
	if hasExpr && !isStr {
		return apierror.BadRequest("CRON", "config_json cron_expression must be a string")
	}
	if st, _ := cfg["schedule_type"].(string); st == "cron" && strings.TrimSpace(exprStr) == "" {
		return apierror.BadRequest("CRON", "config_json cron_expression cannot be empty")
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
