package service

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
)

type monitorBudgetAlertNotifier struct {
	mon *biz.MonitorUsecase
}

func NewMonitorBudgetAlertNotifier(mon *biz.MonitorUsecase) biz.UsageAlertNotifier {
	if mon == nil {
		return nil
	}
	return &monitorBudgetAlertNotifier{mon: mon}
}

func (n *monitorBudgetAlertNotifier) NotifyBudgetAlert(ctx context.Context, alert biz.BudgetAlert, spentMicroUSD, capMicroUSD int64, utilization float64) error {
	if n == nil || n.mon == nil {
		return nil
	}
	meta, _ := json.Marshal(map[string]any{
		"scope_type":      alert.ScopeType,
		"scope_id":        alert.ScopeID,
		"alert_ratio":     alert.AlertRatio,
		"spent_micro_usd": spentMicroUSD,
		"cap_micro_usd":   capMicroUSD,
		"utilization":     utilization,
	})
	return n.mon.RecordMonitorEvent(ctx, biz.MonitorEventWrite{
		EventKey:     "usage.budget_alert",
		Name:         fmt.Sprintf("用量达预算 %.0f%%", alert.AlertRatio*100),
		Description:  fmt.Sprintf("scope=%s/%s spent=%d cap=%d", alert.ScopeType, alert.ScopeID, spentMicroUSD, capMicroUSD),
		Status:       "warn",
		MetadataJSON: string(meta),
	})
}
