package biz

import "aranea-agents/internal/biz/cron"

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
)
