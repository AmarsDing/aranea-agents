package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/errors"
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

// CronTask is one row of cron_task.
type CronTask struct {
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

// CronTaskRun is one row of cron_task_run plus joined task display name and parsed output fields.
type CronTaskRun struct {
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

// CronTaskRunQuery filters ListCronTaskRuns (legacy query keys: task id, status, limit).
type CronTaskRunQuery struct {
	TaskID string
	Status string
	Limit  int
}

type CronTaskRunInput struct {
	ID         string
	TaskID     string
	Status     string
	StartedAt  string
	OutputJSON string
	CreatedAt  string
}

type CronRepo interface {
	ListCronTasks(ctx context.Context) ([]CronTask, error)
	GetCronTask(ctx context.Context, id string) (CronTask, error)
	CreateCronTask(ctx context.Context, t CronTask) (CronTask, error)
	UpdateCronTask(ctx context.Context, t CronTask) (CronTask, error)
	DeleteCronTask(ctx context.Context, id string) error
	ListCronTaskRuns(ctx context.Context, q CronTaskRunQuery) ([]CronTaskRun, error)
	InsertCronTaskRun(ctx context.Context, in CronTaskRunInput) error
	UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error
}

type CronTaskPatch struct {
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

func StrPtr(s string) *string { return &s }
func BoolPtr(b bool) *bool    { return &b }
func IntPtr(i int) *int       { return &i }

type CronUsecase struct {
	repo CronRepo
}

func NewCronUsecase(repo CronRepo) *CronUsecase {
	return &CronUsecase{repo: repo}
}

func (u *CronUsecase) ListTasks(ctx context.Context) ([]CronTask, error) {
	return u.repo.ListCronTasks(ctx)
}

func (u *CronUsecase) GetTask(ctx context.Context, id string) (CronTask, error) {
	if strings.TrimSpace(id) == "" {
		return CronTask{}, errors.BadRequest("CRON", "id is required")
	}
	return u.repo.GetCronTask(ctx, id)
}

func (u *CronUsecase) CreateTask(ctx context.Context, in CronTask) (CronTask, error) {
	in.TaskKey = strings.TrimSpace(in.TaskKey)
	in.Name = strings.TrimSpace(in.Name)
	if in.TaskKey == "" || in.Name == "" {
		return CronTask{}, errors.BadRequest("CRON", "task_key and name are required")
	}
	if in.ID == "" {
		in.ID = newCronTaskID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.repo.CreateCronTask(ctx, in)
}

func (u *CronUsecase) UpdateTask(ctx context.Context, id string, patch CronTaskPatch) (CronTask, error) {
	if strings.TrimSpace(id) == "" {
		return CronTask{}, errors.BadRequest("CRON", "id is required")
	}
	cur, err := u.repo.GetCronTask(ctx, id)
	if err != nil {
		return CronTask{}, err
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
	if patch.Description != nil {
		merged.Description = *patch.Description
	}
	if patch.Enabled != nil {
		merged.Enabled = *patch.Enabled
	}
	if patch.SortOrder != nil {
		merged.SortOrder = *patch.SortOrder
	}
	if patch.AgentID != nil {
		merged.AgentID = *patch.AgentID
	}
	if patch.ConfigJSON != nil {
		merged.ConfigJSON = *patch.ConfigJSON
	}
	if patch.MetadataJSON != nil {
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
	return u.repo.UpdateCronTask(ctx, merged)
}

func (u *CronUsecase) DeleteTask(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("CRON", "id is required")
	}
	return u.repo.DeleteCronTask(ctx, id)
}

func (u *CronUsecase) ListTaskRuns(ctx context.Context, q CronTaskRunQuery) ([]CronTaskRun, error) {
	return u.repo.ListCronTaskRuns(ctx, q)
}
