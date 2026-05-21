package biz

import "errors"

var (
	// ErrCronRunnerDisabled is returned when cron runner is not wired (CRON_RUNNER_DISABLED).
	ErrCronRunnerDisabled = errors.New("cron runner disabled")
	// ErrCronTaskDeleted is returned when operating on a soft-deleted task.
	ErrCronTaskDeleted = errors.New("cron task deleted")
	// ErrCronSessionBusy is returned when the target session already has an active run.
	ErrCronSessionBusy = errors.New("cron session has active run")
)
