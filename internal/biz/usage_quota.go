package biz

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// UsageQuota is a monthly spend cap for a scope (agent / user / global).
type UsageQuota struct {
	ID              string
	ScopeType       string
	ScopeID         string
	MonthlyMicroUSD int64
	PeriodStart     string
	PeriodEnd       string
	CreatedAt       string
	UpdatedAt       string
}

type UsageQuotaCheck struct {
	Quota              UsageQuota
	Allowed            bool
	SpentMicroUSD      int64
	RemainingMicroUSD  int64
	Reason             string
}

func (u *UsageUsecase) GetQuota(ctx context.Context, scopeType, scopeID string) (UsageQuota, error) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return UsageQuota{}, errors.NotFound("USAGE_QUOTA", "quota not configured")
		}
		return UsageQuota{}, err
	}
	return q, nil
}

func (u *UsageUsecase) SetQuota(ctx context.Context, quota UsageQuota) (UsageQuota, error) {
	if strings.TrimSpace(quota.ScopeType) == "" || strings.TrimSpace(quota.ScopeID) == "" {
		return UsageQuota{}, errors.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	if quota.MonthlyMicroUSD < 0 {
		return UsageQuota{}, errors.BadRequest("USAGE_QUOTA", "monthly_micro_usd must be >= 0")
	}
	return u.repo.SetQuota(ctx, quota)
}

// CheckQuota returns whether another chat turn is allowed under the configured cap.
func (u *UsageUsecase) CheckQuota(ctx context.Context, scopeType, scopeID string) (UsageQuotaCheck, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return UsageQuotaCheck{}, errors.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return UsageQuotaCheck{Allowed: true, Reason: "no quota configured"}, nil
		}
		return UsageQuotaCheck{}, err
	}
	if q.MonthlyMicroUSD <= 0 {
		return UsageQuotaCheck{Quota: q, Allowed: true, Reason: "quota disabled"}, nil
	}
	spent, err := u.quotaSpent(ctx, scopeType, scopeID, q)
	if err != nil {
		return UsageQuotaCheck{}, err
	}
	remaining := q.MonthlyMicroUSD - spent
	if remaining < 0 {
		remaining = 0
	}
	check := UsageQuotaCheck{
		Quota:             q,
		SpentMicroUSD:     spent,
		RemainingMicroUSD: remaining,
	}
	if spent >= q.MonthlyMicroUSD {
		check.Allowed = false
		check.Reason = fmt.Sprintf("monthly quota exceeded: spent %d >= cap %d micro-USD", spent, q.MonthlyMicroUSD)
		return check, nil
	}
	check.Allowed = true
	check.Reason = "within quota"
	return check, nil
}

func (u *UsageUsecase) quotaSpent(ctx context.Context, scopeType, scopeID string, q UsageQuota) (int64, error) {
	switch scopeType {
	case "agent":
		return u.repo.SumAgentCostInPeriod(ctx, scopeID, q.PeriodStart, q.PeriodEnd)
	default:
		return 0, fmt.Errorf("unsupported quota scope_type %q", scopeType)
	}
}
