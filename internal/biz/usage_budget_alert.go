package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/errors"
)

// BudgetAlert is a spend-ratio threshold for a scope.
type BudgetAlert struct {
	ID          string
	ScopeType   string
	ScopeID     string
	AlertRatio  float64
	Enabled     bool
	LastFiredAt string
	CreatedAt   string
	UpdatedAt   string
}

// UsageAlertNotifier delivers budget threshold notifications (monitor / WS).
type UsageAlertNotifier interface {
	NotifyBudgetAlert(ctx context.Context, alert BudgetAlert, spentMicroUSD, capMicroUSD int64, utilization float64) error
}

// QuotaDashboard summarizes agent quota utilization for the overview page.
type QuotaDashboard struct {
	ConfiguredCount    int
	TotalCapMicroUSD   int64
	TotalSpentMicroUSD int64
	MaxUtilization     float64
}

var alertCooldown = 60 * time.Minute

// SetAlertNotifier wires optional budget alert delivery.
func (u *UsageUsecase) SetAlertNotifier(n UsageAlertNotifier) {
	u.alertNotifier = n
}

func (u *UsageUsecase) scheduleBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
	if u == nil || u.alertNotifier == nil || e.TotalCostMicroUSD <= 0 {
		return
	}
	if strings.TrimSpace(e.AgentID) == "" && strings.TrimSpace(e.UserID) == "" {
		return
	}
	ev := e
	safego.Go(ctx, "usage.budget_alert", func() {
		u.EvaluateBudgetAlerts(context.WithoutCancel(ctx), ev)
	})
}

func (u *UsageUsecase) EvaluateBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
	if u == nil || u.alertNotifier == nil {
		return
	}
	if id := strings.TrimSpace(e.AgentID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "agent", id)
	}
	if id := strings.TrimSpace(e.UserID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "user", id)
	}
	u.evaluateBudgetAlertsForScope(ctx, "global", "global")
}

func (u *UsageUsecase) evaluateBudgetAlertsForScope(ctx context.Context, scopeType, scopeID string) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil || q.MonthlyMicroUSD <= 0 {
		return
	}
	spent, err := u.repo.SumScopeCostInPeriod(ctx, scopeType, scopeID, q.PeriodStart, q.PeriodEnd)
	if err != nil || spent <= 0 {
		return
	}
	util := float64(spent) / float64(q.MonthlyMicroUSD)
	alerts, err := u.repo.ListBudgetAlerts(ctx, scopeType, scopeID)
	if err != nil {
		return
	}
	now := u.now().UTC()
	for _, a := range alerts {
		if !a.Enabled || a.AlertRatio <= 0 || util+1e-9 < a.AlertRatio {
			continue
		}
		if u.alertRecentlyFired(a, now) {
			continue
		}
		if err := u.alertNotifier.NotifyBudgetAlert(ctx, a, spent, q.MonthlyMicroUSD, util); err != nil {
			continue
		}
		_ = u.repo.UpdateBudgetAlertLastFired(ctx, a.ID, now.Format(time.RFC3339))
		u.markAlertFired(a.ID, now)
	}
}

func (u *UsageUsecase) alertRecentlyFired(a BudgetAlert, now time.Time) bool {
	u.alertFiredMu.Lock()
	defer u.alertFiredMu.Unlock()
	if u.alertFired == nil {
		u.alertFired = make(map[string]time.Time)
	}
	if t, ok := u.alertFired[a.ID]; ok && now.Sub(t) < alertCooldown {
		return true
	}
	if strings.TrimSpace(a.LastFiredAt) != "" {
		if t, err := time.Parse(time.RFC3339, a.LastFiredAt); err == nil && now.Sub(t) < alertCooldown {
			return true
		}
	}
	return false
}

func (u *UsageUsecase) markAlertFired(id string, now time.Time) {
	u.alertFiredMu.Lock()
	defer u.alertFiredMu.Unlock()
	if u.alertFired == nil {
		u.alertFired = make(map[string]time.Time)
	}
	u.alertFired[id] = now
}

func (u *UsageUsecase) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error) {
	return u.repo.ListBudgetAlerts(ctx, scopeType, scopeID)
}

func (u *UsageUsecase) SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error) {
	if strings.TrimSpace(alert.ScopeType) == "" || strings.TrimSpace(alert.ScopeID) == "" {
		return BudgetAlert{}, errors.BadRequest("USAGE_ALERT", "scope_type and scope_id are required")
	}
	if alert.AlertRatio <= 0 || alert.AlertRatio > 1 {
		return BudgetAlert{}, errors.BadRequest("USAGE_ALERT", "alert_ratio must be in (0,1]")
	}
	a, err := u.repo.SetBudgetAlert(ctx, alert)
	return a, mapUsageRepoErr(err)
}

func (u *UsageUsecase) QuotaDashboard(ctx context.Context) (QuotaDashboard, error) {
	quotas, err := u.repo.ListActiveQuotas(ctx)
	if err != nil {
		return QuotaDashboard{}, err
	}
	var dash QuotaDashboard
	var maxUtil float64
	for _, q := range quotas {
		if q.MonthlyMicroUSD <= 0 {
			continue
		}
		spent, err := u.repo.SumScopeCostInPeriod(ctx, q.ScopeType, q.ScopeID, q.PeriodStart, q.PeriodEnd)
		if err != nil {
			continue
		}
		dash.ConfiguredCount++
		dash.TotalCapMicroUSD += q.MonthlyMicroUSD
		dash.TotalSpentMicroUSD += spent
		util := float64(spent) / float64(q.MonthlyMicroUSD)
		if util > maxUtil {
			maxUtil = util
		}
	}
	dash.MaxUtilization = maxUtil
	return dash, nil
}

// ExportUsageEventsCSV returns CSV rows for usage events (header + data).
func (u *UsageUsecase) ExportUsageEventsCSV(ctx context.Context, query UsageQuery) (string, error) {
	query.Limit = 5000
	if query.Limit <= 0 {
		query.Limit = 5000
	}
	events, err := u.Events(ctx, query)
	if err != nil {
		return "", err
	}
	return formatUsageEventsCSV(events), nil
}
