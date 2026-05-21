package service

import (
	"database/sql"
	"errors"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func mapCronError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return kerrors.NotFound("CRON", "cron task not found")
	}
	if errors.Is(err, biz.ErrCronRunnerDisabled) {
		return kerrors.ServiceUnavailable("CRON", "cron runner is disabled")
	}
	if errors.Is(err, biz.ErrCronTaskDeleted) {
		return kerrors.NotFound("CRON", "cron task deleted")
	}
	if errors.Is(err, biz.ErrCronSessionBusy) {
		return kerrors.New(409, "CRON_SESSION_BUSY", err.Error())
	}
	var ke *kerrors.Error
	if errors.As(err, &ke) {
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		return kerrors.BadRequest("CRON", err.Error())
	}
	return err
}
