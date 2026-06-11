package service

import (
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func mapCronError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, biz.ErrCronNotFound) {
		return apierror.NotFound("CRON", "cron task not found")
	}
	if errors.Is(err, biz.ErrCronRunnerDisabled) {
		return apierror.Unavailable("CRON", "cron runner is disabled")
	}
	if errors.Is(err, biz.ErrCronTaskDeleted) {
		return apierror.NotFound("CRON", "cron task deleted")
	}
	if errors.Is(err, biz.ErrCronSessionBusy) {
		return apierror.Conflict("CRON", err.Error())
	}
	if ae, ok := apierror.From(err); ok {
		return ae
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		return apierror.BadRequest("CRON", err.Error())
	}
	return err
}
