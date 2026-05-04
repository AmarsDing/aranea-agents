package cronrunner

import "errors"

var (
	errRunAtRequired = errors.New("run_at is required for once cron task")
	errInvalidRunAt  = errors.New("invalid run_at")
	errCronFields    = errors.New("cron expression must have 5 fields")
	errCronNoSlot    = errors.New("unable to find next cron time within one year")
)
