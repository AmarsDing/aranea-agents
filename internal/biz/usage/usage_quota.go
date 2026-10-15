package usage

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// Stable quota-check reason codes (API contract; UI maps them to localized text).
const (
	QuotaCheckReasonNoQuota  = "no_quota"
	QuotaCheckReasonDisabled = "quota_disabled"
	QuotaCheckReasonExceeded = "quota_exceeded"
	QuotaCheckReasonWithin   = "within_quota"
)

// QuotaExceededMessage builds the localized user-facing message for a blocked turn.
func QuotaExceededMessage(check QuotaCheck) string {
	return fmt.Sprintf("本月用量已达配额上限（已消耗 $%.2f / 上限 $%.2f），新对话已被拦截；请调整配额或等待下月自动重置",
		float64(check.SpentMicroUSD)/1e6, float64(check.Quota.MonthlyMicroUSD)/1e6)
}

// GetQuota returns the quota for a scope.
func (u *Usecase) GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return Quota{}, apierror.NotFound("USAGE_QUOTA", "quota not configured")
		}
		return Quota{}, MapRepoErr(err)
	}
	return q, nil
}

// SetQuota creates or updates a quota.
func (u *Usecase) SetQuota(ctx context.Context, quota Quota) (Quota, error) {
	if strings.TrimSpace(quota.ScopeType) == "" || strings.TrimSpace(quota.ScopeID) == "" {
		return Quota{}, apierror.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	if quota.MonthlyMicroUSD < 0 {
		return Quota{}, apierror.BadRequest("USAGE_QUOTA", "monthly_micro_usd must be >= 0")
	}
	if err := validateQuotaPeriod(quota.PeriodStart, quota.PeriodEnd); err != nil {
		return Quota{}, err
	}
	q, err := u.repo.SetQuota(ctx, quota)
	return q, MapRepoErr(err)
}

// validateQuotaPeriod rejects malformed period bounds. Both empty is allowed
// (the data layer defaults to the current calendar month); exactly one set is
// ambiguous and rejected. When both are set they must be YYYY-MM-DD and
// start <= end.
func validateQuotaPeriod(periodStart, periodEnd string) error {
	start := strings.TrimSpace(periodStart)
	end := strings.TrimSpace(periodEnd)
	if start == "" && end == "" {
		return nil
	}
	if start == "" || end == "" {
		return apierror.BadRequest("USAGE_QUOTA", "period_start and period_end must be set together")
	}
	if _, err := time.Parse("2006-01-02", start); err != nil {
		return apierror.BadRequest("USAGE_QUOTA", "period_start must be YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", end); err != nil {
		return apierror.BadRequest("USAGE_QUOTA", "period_end must be YYYY-MM-DD")
	}
	if start > end {
		return apierror.BadRequest("USAGE_QUOTA", "period_start must be <= period_end")
	}
	return nil
}

// CheckQuota returns whether another chat turn is allowed under the configured cap.
func (u *Usecase) CheckQuota(ctx context.Context, scopeType, scopeID string) (QuotaCheck, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return QuotaCheck{}, apierror.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return QuotaCheck{Allowed: true, Reason: QuotaCheckReasonNoQuota}, nil
		}
		return QuotaCheck{}, err
	}
	if q.MonthlyMicroUSD <= 0 {
		return QuotaCheck{Quota: q, Allowed: true, Reason: QuotaCheckReasonDisabled}, nil
	}
	q = u.normalizeQuotaPeriod(ctx, q)
	spent, err := u.quotaSpent(ctx, scopeType, scopeID, q)
	if err != nil {
		return QuotaCheck{}, err
	}
	remaining := q.MonthlyMicroUSD - spent
	if remaining < 0 {
		remaining = 0
	}
	check := QuotaCheck{
		Quota:             q,
		SpentMicroUSD:     spent,
		RemainingMicroUSD: remaining,
	}
	if spent >= q.MonthlyMicroUSD {
		check.Allowed = false
		check.Reason = QuotaCheckReasonExceeded
		return check, nil
	}
	check.Allowed = true
	check.Reason = QuotaCheckReasonWithin
	return check, nil
}

// normalizeQuotaPeriod lazily rolls an expired monthly period forward to the
// current UTC calendar month (same convention as date_key) and best-effort
// persists it; on persist failure the in-memory period is still used so
// enforcement never silently keeps summing a stale month.
func (u *Usecase) normalizeQuotaPeriod(ctx context.Context, q Quota) Quota {
	if !quotaPeriodExpired(q.PeriodEnd, u.now()) {
		return q
	}
	rolled := q
	rolled.PeriodStart, rolled.PeriodEnd = currentMonthPeriod(u.now())
	if _, err := u.repo.SetQuota(ctx, rolled); err != nil {
		u.lg.Warn("quota period rollover persist failed; enforcing with in-memory period",
			loggateway.Err(err),
			loggateway.Str("scope_type", q.ScopeType),
			loggateway.Str("scope_id", q.ScopeID))
		return rolled
	}
	u.lg.Info("quota period rolled to current month",
		loggateway.Str("scope_type", q.ScopeType),
		loggateway.Str("scope_id", q.ScopeID),
		loggateway.Str("period_start", rolled.PeriodStart),
		loggateway.Str("period_end", rolled.PeriodEnd))
	return rolled
}

// quotaPeriodExpired reports whether the configured period no longer covers
// today (UTC). Empty or malformed periods count as expired so they self-heal.
func quotaPeriodExpired(periodEnd string, now time.Time) bool {
	end := strings.TrimSpace(periodEnd)
	if _, err := time.Parse("2006-01-02", end); err != nil {
		return true
	}
	return end < now.Format("2006-01-02")
}

// currentMonthPeriod returns the first and last day ("YYYY-MM-DD") of the
// calendar month containing now.
func currentMonthPeriod(now time.Time) (string, string) {
	y, m, _ := now.Date()
	start := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

func (u *Usecase) quotaSpent(ctx context.Context, scopeType, scopeID string, q Quota) (int64, error) {
	switch scopeType {
	case "agent", "user", "global":
		spent, err := u.repo.SumScopeCostInPeriod(ctx, scopeType, scopeID, q.PeriodStart, q.PeriodEnd)
		return spent, MapRepoErr(err)
	default:
		return 0, shared.ErrQuotaUnsupportedScope
	}
}

// CheckTeamMemberQuotas validates that all enabled team members are within quota.
// Returns nil when teamID is empty or no team reader is configured.
// Uses batch queries to avoid N+1 (S3 fix).
func (u *Usecase) CheckTeamMemberQuotas(ctx context.Context, teamID string) error {
	if u == nil || u.teamReader == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	agentIDs, err := u.teamReader.EnabledMemberAgentIDs(ctx, teamID)
	if err != nil {
		return err
	}
	if len(agentIDs) == 0 {
		return nil
	}

	allQuotas, err := u.repo.ListActiveQuotas(ctx)
	if err != nil {
		return err
	}

	agentIDSet := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		agentIDSet[id] = struct{}{}
	}
	relevant := make([]Quota, 0, len(agentIDs))
	for _, q := range allQuotas {
		if q.ScopeType != "agent" {
			continue
		}
		if _, ok := agentIDSet[q.ScopeID]; !ok {
			continue
		}
		if q.MonthlyMicroUSD <= 0 {
			continue
		}
		relevant = append(relevant, u.normalizeQuotaPeriod(ctx, q))
	}
	if len(relevant) == 0 {
		return nil
	}

	spentMap, err := u.repo.BatchSumScopeCost(ctx, relevant)
	if err != nil {
		return err
	}
	for _, q := range relevant {
		key := q.ScopeType + ":" + q.ScopeID
		spent := spentMap[key]
		if spent >= q.MonthlyMicroUSD {
			return apierror.Forbidden("USAGE_QUOTA",
				"成员 Agent「%s」本月用量已达配额上限（已消耗 $%.2f / 上限 $%.2f），团队运行已被拦截",
				q.ScopeID, float64(spent)/1e6, float64(q.MonthlyMicroUSD)/1e6)
		}
	}
	return nil
}

// enforceQuota blocks when the scope monthly cap is exceeded (no-op if quota unset).
func (u *Usecase) enforceQuota(ctx context.Context, scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	check, err := u.CheckQuota(ctx, scopeType, scopeID)
	if err != nil {
		return err
	}
	if !check.Allowed {
		return apierror.Forbidden("USAGE_QUOTA", QuotaExceededMessage(check))
	}
	return nil
}
