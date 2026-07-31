package a2a

import (
	"context"
	"strconv"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// FederationLimiterFactory builds a per-policy per-minute sliding-window
// Limiter with the given max. Injected at the composition root so the storage
// backend (Redis vs in-memory) stays a wire-time decision, mirroring
// NewLimiter: production wraps NewLimiter with LimiterConfig{WindowSize:
// time.Minute, MaxInvokes: maxPerMin, KeyPrefix: <fed prefix>}.
type FederationLimiterFactory func(maxPerMin int) Limiter

// QuotaChecker enforces per-pair quotas from the explicit policy
// (design F.5):
//   - MaxPerMin > 0: sliding-window per-minute cap via a per-pair Limiter
//     (caller key prefixed "fed:").
//   - DailyQuota > 0: count of decision=allowed audits since UTC day start
//     must stay below the quota.
//   - Both 0 (or no explicit policy): allow.
//
// Both backends are fail-closed: a limiter/count error blocks the call with a
// 500-class error rather than silently skipping governance.
type QuotaChecker struct {
	mu       sync.Mutex
	limiters map[string]Limiter
	factory  FederationLimiterFactory
	engine   *PolicyEngine
	audits   FederationAuditRepo
	lg       loggateway.Logger
	now      func() time.Time
}

// NewQuotaChecker constructs a QuotaChecker. factory may be nil when no
// policy uses MaxPerMin (memory-only tests); a MaxPerMin policy evaluated
// against a nil factory fails closed with a 500 rather than panicking.
func NewQuotaChecker(engine *PolicyEngine, audits FederationAuditRepo, factory FederationLimiterFactory, lg loggateway.Logger) *QuotaChecker {
	return &QuotaChecker{
		limiters: make(map[string]Limiter),
		factory:  factory,
		engine:   engine,
		audits:   audits,
		lg:       lg,
		now:      time.Now,
	}
}

// limiterFor returns the cached per-pair limiter, building one when the pair
// or its MaxPerMin changed (policy updates reset the window state). A nil
// factory with a MaxPerMin policy is a composition-root misconfiguration;
// fail closed (nil, false) rather than panic so Check returns a 500.
func (q *QuotaChecker) limiterFor(callerOrgID, calleeOrgID string, maxPerMin int) (Limiter, bool) {
	if q.factory == nil {
		return nil, false
	}
	key := callerOrgID + "\x00" + calleeOrgID + "\x00" + strconv.Itoa(maxPerMin)
	q.mu.Lock()
	defer q.mu.Unlock()
	if lim, ok := q.limiters[key]; ok {
		return lim, true
	}
	lim := q.factory(maxPerMin)
	q.limiters[key] = lim
	return lim, true
}

// utcDayStart returns the start of the current UTC day — the daily-quota window.
func (q *QuotaChecker) utcDayStart() time.Time {
	n := q.now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// Check reports whether a call for the org pair is within quota.
// Returns nil when allowed; *apierror.Error with CodeRateLimit (HTTP 429)
// when a quota is exceeded; a 500-class error when a backend fails.
func (q *QuotaChecker) Check(ctx context.Context, callerOrgID, calleeOrgID string) error {
	p, found := q.engine.Evaluate(callerOrgID, calleeOrgID)
	if !found {
		return nil
	}
	// Denied pairs are rejected at the policy step before quota runs; never
	// consume quota for them.
	if normalizeAction(p.Action) == PolicyActionDeny {
		return nil
	}
	if p.MaxPerMin > 0 {
		lim, ok := q.limiterFor(callerOrgID, calleeOrgID, p.MaxPerMin)
		if !ok {
			return apierror.Internal(apierror.DomainA2AFed, "federation per-minute limiter not configured (nil factory) for %s -> %s", callerOrgID, calleeOrgID)
		}
		ok, err := lim.Allow(ctx, "fed:"+callerOrgID, calleeOrgID)
		if err != nil {
			return apierror.Internal(apierror.DomainA2AFed, "federation per-minute limiter unavailable: %v", err).WithCause(err)
		}
		if !ok {
			if q.lg != nil {
				q.lg.Warn("federation call denied by per-minute quota",
					loggateway.StepID("a2a.fed.quota.denied"),
					loggateway.Str("policy_id", p.ID),
					loggateway.Str("caller_org_id", callerOrgID),
					loggateway.Str("callee_org_id", calleeOrgID),
					loggateway.Int("max_per_min", p.MaxPerMin),
				)
			}
			return apierror.RateLimit(apierror.DomainA2AFed, "federation per-minute quota exceeded for %s -> %s (max %d/min)", callerOrgID, calleeOrgID, p.MaxPerMin)
		}
	}
	if p.DailyQuota > 0 {
		n, err := q.audits.CountCallsSince(ctx, callerOrgID, calleeOrgID, q.utcDayStart())
		if err != nil {
			return apierror.Internal(apierror.DomainA2AFed, "federation daily quota count unavailable: %v", err).WithCause(err)
		}
		if n >= p.DailyQuota {
			if q.lg != nil {
				q.lg.Warn("federation call denied by daily quota",
					loggateway.StepID("a2a.fed.quota.denied"),
					loggateway.Str("policy_id", p.ID),
					loggateway.Str("caller_org_id", callerOrgID),
					loggateway.Str("callee_org_id", calleeOrgID),
					loggateway.Int("daily_quota", p.DailyQuota),
				)
			}
			return apierror.RateLimit(apierror.DomainA2AFed, "federation daily quota exceeded for %s -> %s (max %d/day)", callerOrgID, calleeOrgID, p.DailyQuota)
		}
	}
	return nil
}
