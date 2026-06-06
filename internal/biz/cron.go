package biz

import (
	"context"

	"aranea-agents/internal/biz/cron"
)

type CronTriggerGateway interface {
	TriggerCronTask(ctx context.Context, taskID string) (CronTaskRun, error)
	GetTaskRun(ctx context.Context, id string) (CronTaskRun, error)
}

type (
	CronTask         = cron.Task
	CronTaskRun      = cron.TaskRun
	CronTaskRunQuery = cron.TaskRunQuery
	CronTaskRunInput = cron.TaskRunInput
	CronTaskPatch    = cron.TaskPatch
	CronRepo         = cron.Repo
	CronTaskTrigger  = cron.TaskTrigger
	CronUsecase      = cron.Usecase
)

var (
	NewCronUsecase           = cron.NewUsecase
	ResetCronFailureMetadata = cron.ResetFailureMetadata
	StrPtr                   = cron.StrPtr
	BoolPtr                  = cron.BoolPtr
	IntPtr                   = cron.IntPtr
	ErrCronRunnerDisabled    = cron.ErrRunnerDisabled
	ErrCronTaskDeleted       = cron.ErrTaskDeleted
	ErrCronSessionBusy       = cron.ErrSessionBusy
	ErrCronNotFound          = cron.ErrNotFound
)
