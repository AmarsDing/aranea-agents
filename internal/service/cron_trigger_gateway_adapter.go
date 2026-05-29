package service

import (
	"context"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type cronTriggerGatewayAdapter struct {
	svc *CronService
}

func NewCronTriggerGatewayAdapter(svc *CronService) biz.CronTriggerGateway {
	return cronTriggerGatewayAdapter{svc: svc}
}

func (a cronTriggerGatewayAdapter) TriggerCronTask(ctx context.Context, taskID string) (biz.CronTaskRun, error) {
	if a.svc == nil || a.svc.uc == nil {
		return biz.CronTaskRun{}, kerrors.InternalServer("CRON", "cron service not configured")
	}
	return a.svc.uc.TriggerTask(ctx, taskID)
}

func (a cronTriggerGatewayAdapter) GetTaskRun(ctx context.Context, id string) (biz.CronTaskRun, error) {
	return a.svc.GetTaskRun(ctx, id)
}
