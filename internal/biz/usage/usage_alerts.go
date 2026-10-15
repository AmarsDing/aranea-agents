package usage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// QuotaDashboard summarizes agent quota utilization for the overview page.
type QuotaDashboard struct {
	ConfiguredCount    int
	TotalCapMicroUSD   int64
	TotalSpentMicroUSD int64
	MaxUtilization     float64
}

var alertCooldown = 60 * time.Minute

func (u *Usecase) scheduleBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
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

// EvaluateBudgetAlerts checks budget thresholds after a usage event.
func (u *Usecase) EvaluateBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
	if u == nil || u.alertNotifier == nil {
		return
	}
	if id := strings.TrimSpace(e.AgentID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "agent", id)
	}
	if id := strings.TrimSpace(e.UserID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "user", id)
	}
	u.evaluateBudgetAlertsForScope(ctx, QuotaScopeGlobal, GlobalQuotaScopeID)
}

func (u *Usecase) evaluateBudgetAlertsForScope(ctx context.Context, scopeType, scopeID string) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil || q.MonthlyMicroUSD <= 0 {
		return
	}
	q = u.normalizeQuotaPeriod(ctx, q)
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
		if !a.Enabled || a.AlertRatio <= 0 || util+floatCompareEpsilon < a.AlertRatio {
			continue
		}
		if u.alertRecentlyFired(a, now) {
			continue
		}
		if err := u.alertNotifier.NotifyBudgetAlert(ctx, a, spent, q.MonthlyMicroUSD, util); err != nil {
			continue
		}
		if err := u.repo.UpdateBudgetAlertLastFired(ctx, a.ID, now.Format(time.RFC3339)); err != nil {
			u.lg.Warn("update budget alert last fired failed", loggateway.Err(err), loggateway.Str("alert_id", a.ID))
		}
		u.markAlertFired(a.ID, now)
	}
}

func (u *Usecase) alertRecentlyFired(a BudgetAlert, now time.Time) bool {
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

func (u *Usecase) markAlertFired(id string, now time.Time) {
	u.alertFiredMu.Lock()
	defer u.alertFiredMu.Unlock()
	if u.alertFired == nil {
		u.alertFired = make(map[string]time.Time)
	}
	u.alertFired[id] = now
}

// ListBudgetAlerts returns budget alerts for a scope.
func (u *Usecase) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error) {
	return u.repo.ListBudgetAlerts(ctx, scopeType, scopeID)
}

// SetBudgetAlert creates or updates a budget alert.
func (u *Usecase) SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error) {
	if strings.TrimSpace(alert.ScopeType) == "" || strings.TrimSpace(alert.ScopeID) == "" {
		return BudgetAlert{}, apierror.BadRequest("USAGE_ALERT", "scope_type and scope_id are required")
	}
	if alert.AlertRatio <= 0 || alert.AlertRatio > 1 {
		return BudgetAlert{}, apierror.BadRequest("USAGE_ALERT", "alert_ratio must be in (0,1]")
	}
	a, err := u.repo.SetBudgetAlert(ctx, alert)
	return a, MapRepoErr(err)
}

// QuotaDashboard summarizes agent quota utilization for the overview page.
func (u *Usecase) QuotaDashboard(ctx context.Context) (QuotaDashboard, error) {
	quotas, err := u.repo.ListActiveQuotas(ctx)
	if err != nil {
		return QuotaDashboard{}, err
	}
	var dash QuotaDashboard
	if len(quotas) == 0 {
		return dash, nil
	}
	spentMap, batchErr := u.repo.BatchSumScopeCost(ctx, quotas)
	if batchErr != nil {
		u.lg.Warn("quota_dashboard.batch_failed", loggateway.StepID("usage"), loggateway.Err(batchErr))
	}
	var maxUtil float64
	for _, q := range quotas {
		if q.MonthlyMicroUSD <= 0 {
			continue
		}
		key := q.ScopeType + ":" + q.ScopeID
		spent, ok := spentMap[key]
		if !ok && batchErr != nil {
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

const (
	inefficientMinCalls       = 3
	inefficientCostMicroFloor = int64(100_000)
	inefficientLowTPS         = 5.0
	inefficientLowSuccess     = 0.85
)

// InefficientModels returns top models in range that match high-cost + (low TPS or low success).
func (u *Usecase) InefficientModels(ctx context.Context, query Query) ([]ModelInsight, error) {
	q := withLimit(u.normalizeQuery(query, u.now()), inefficientModelLimit)
	var rows []BreakdownRow
	var err error

	if u.dailySupported(q) && q.EndDate < dateKey(u.now()) {
		rows, err = u.repo.ListTopModelUsageFromDaily(ctx, q)
	} else {
		rows, err = u.repo.ListTopModelUsage(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	var out []ModelInsight
	for _, r := range rows {
		if r.CallCount < inefficientMinCalls || r.TotalCostMicroUSD < inefficientCostMicroFloor {
			continue
		}
		var flags []string
		if r.AvgTokensPerSecond > 0 && r.AvgTokensPerSecond < inefficientLowTPS {
			flags = append(flags, "low_tps")
		}
		if r.SuccessRate > 0 && r.SuccessRate < inefficientLowSuccess {
			flags = append(flags, "high_failure")
		}
		if r.TotalCostMicroUSD >= inefficientCostMicroFloor*10 {
			flags = append(flags, "high_cost")
		}
		if len(flags) == 0 {
			continue
		}
		out = append(out, ModelInsight{
			ProviderCode:       r.ProviderCode,
			ModelAPIID:         r.ModelAPIID,
			ModelDisplayName:   strings.TrimSpace(r.ModelDisplayName),
			CallCount:          r.CallCount,
			TotalTokens:        r.TotalTokens,
			TotalCostMicroUSD:  r.TotalCostMicroUSD,
			AvgLatencyMS:       r.AvgLatencyMS,
			AvgTokensPerSecond: r.AvgTokensPerSecond,
			SuccessRate:        r.SuccessRate,
			Flags:              flags,
		})
		if len(out) >= inefficientResultLimit {
			break
		}
	}
	return out, nil
}

// ExportUsageEventsCSV returns CSV rows for usage events.
func (u *Usecase) ExportUsageEventsCSV(ctx context.Context, query Query) (string, error) {
	if query.Limit <= 0 || query.Limit > csvExportMaxRows {
		query.Limit = csvExportMaxRows
	}
	events, err := u.Events(ctx, query)
	if err != nil {
		return "", err
	}
	return formatUsageEventsCSV(events), nil
}

func formatUsageEventsCSV(events []TokenUsageEvent) string {
	var b strings.Builder
	b.WriteString("occurred_at,usage_kind,agent_id,provider_code,model_api_id,session_id,team_id,input_tokens,output_tokens,total_tokens,total_cost_micro_usd,latency_ms,status,error_message\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%d,%d,%d,%d,%d,%q,%q\n",
			e.OccurredAt, e.UsageKind, e.AgentID, e.ProviderCode, e.ModelAPIID, e.SessionID, e.TeamID,
			e.InputTokens, e.OutputTokens, e.TotalTokens, e.TotalCostMicroUSD, e.LatencyMS, e.Status, csvEscape(e.ErrorMessage),
		))
	}
	return b.String()
}

func csvEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `"`, `""`), "\n", " ")
}
