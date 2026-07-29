package a2a

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type stubLimiter struct {
	allowFn func(ctx context.Context, caller, callee string) (bool, error)
}

func (s stubLimiter) Allow(ctx context.Context, caller, callee string) (bool, error) {
	return s.allowFn(ctx, caller, callee)
}

func newEngineWithPolicy(t *testing.T, p FederationPolicy) *PolicyEngine {
	t.Helper()
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			if p.ID == "" {
				return nil, nil
			}
			return []FederationPolicy{p}, nil
		},
		upsertFn: func(_ context.Context, in FederationPolicy) (FederationPolicy, error) {
			p = in
			return in, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return e
}

func TestQuotaChecker_NoPolicyAllows(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{})
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		return NewMemorySlidingWindowLimiter(LimiterConfig{WindowSize: time.Minute, MaxInvokes: maxPerMin})
	}, loggateway.NewNoop())
	if err := q.Check(context.Background(), FederationLocalOrgID, "org-x"); err != nil {
		t.Fatalf("Check() error: %v, want nil (no explicit policy)", err)
	}
}

func TestQuotaChecker_ZeroLimitsAllow(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 0, DailyQuota: 0,
	})
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, nil, loggateway.NewNoop())
	if err := q.Check(context.Background(), FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("Check() error: %v, want nil (both limits 0)", err)
	}
}

func TestQuotaChecker_DenyPolicySkipsQuota(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionDeny, MaxPerMin: 1,
	})
	called := false
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		called = true
		return NewMemorySlidingWindowLimiter(LimiterConfig{WindowSize: time.Minute, MaxInvokes: maxPerMin})
	}, loggateway.NewNoop())
	// The policy step rejects denied pairs before quota runs; Check must not
	// consume quota for them.
	if err := q.Check(context.Background(), FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("Check() error: %v, want nil (deny handled by policy step)", err)
	}
	if called {
		t.Error("limiter factory called for denied pair, want skipped")
	}
}

func TestQuotaChecker_PerMinLimitEnforced(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 2,
	})
	var gotCaller, gotCallee string
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		if maxPerMin != 2 {
			t.Errorf("factory maxPerMin = %d, want 2", maxPerMin)
		}
		return stubLimiter{allowFn: func(_ context.Context, caller, callee string) (bool, error) {
			gotCaller, gotCallee = caller, callee
			return true, nil
		}}
	}, loggateway.NewNoop())
	if err := q.Check(context.Background(), FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if gotCaller != "fed:"+FederationLocalOrgID {
		t.Errorf("limiter caller = %q, want %q", gotCaller, "fed:"+FederationLocalOrgID)
	}
	if gotCallee != "org-b" {
		t.Errorf("limiter callee = %q, want %q", gotCallee, "org-b")
	}
}

func TestQuotaChecker_PerMinExceededReturns429(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 2,
	})
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		return NewMemorySlidingWindowLimiter(LimiterConfig{WindowSize: time.Minute, MaxInvokes: maxPerMin})
	}, loggateway.NewNoop())
	ctx := context.Background()
	if err := q.Check(ctx, FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("call 1 error: %v", err)
	}
	if err := q.Check(ctx, FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("call 2 error: %v", err)
	}
	err := q.Check(ctx, FederationLocalOrgID, "org-b")
	if err == nil {
		t.Fatal("call 3 expected rate-limit error, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Errorf("error = %v, want code %v (HTTP 429)", err, apierror.CodeRateLimit)
	}
}

func TestQuotaChecker_PerMinLimiterRecreatedOnPolicyChange(t *testing.T) {
	var engine *PolicyEngine
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{{
				ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
				Action: PolicyActionAllow, MaxPerMin: 1,
			}}, nil
		},
		upsertFn: func(_ context.Context, in FederationPolicy) (FederationPolicy, error) {
			in.ID = "p1"
			return in, nil
		},
	}
	engine = NewPolicyEngine(repo, loggateway.NewNoop())
	if err := engine.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	q := NewQuotaChecker(engine, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		return NewMemorySlidingWindowLimiter(LimiterConfig{WindowSize: time.Minute, MaxInvokes: maxPerMin})
	}, loggateway.NewNoop())
	ctx := context.Background()
	if err := q.Check(ctx, FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("call 1 error: %v", err)
	}
	if err := q.Check(ctx, FederationLocalOrgID, "org-b"); !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Fatalf("call 2 expected rate-limit (MaxPerMin=1), got %v", err)
	}
	// Raising MaxPerMin must build a fresh limiter (window state resets).
	if _, err := engine.UpsertPolicy(ctx, FederationPolicy{
		CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 3,
	}); err != nil {
		t.Fatalf("UpsertPolicy() error: %v", err)
	}
	if err := q.Check(ctx, FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("call after quota raise error: %v, want nil (fresh limiter)", err)
	}
}

func TestQuotaChecker_DailyQuotaEnforced(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, DailyQuota: 2,
	})
	audits := &mockFederationAuditRepo{
		countFn: func(context.Context, string, string, time.Time) (int, error) {
			return 2, nil // already at quota
		},
	}
	q := NewQuotaChecker(e, audits, nil, loggateway.NewNoop())
	err := q.Check(context.Background(), FederationLocalOrgID, "org-b")
	if err == nil {
		t.Fatal("Check() expected daily-quota error, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Errorf("error = %v, want code %v (HTTP 429)", err, apierror.CodeRateLimit)
	}
}

func TestQuotaChecker_DailyQuotaWindowIsUTCDay(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, DailyQuota: 10,
	})
	audits := &mockFederationAuditRepo{}
	q := NewQuotaChecker(e, audits, nil, loggateway.NewNoop())
	q.now = func() time.Time { return time.Date(2026, 7, 29, 15, 30, 0, 0, time.UTC) }
	if err := q.Check(context.Background(), FederationLocalOrgID, "org-b"); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	want := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	if !audits.lastSince.Equal(want) {
		t.Errorf("CountCallsSince since = %v, want %v (UTC day start)", audits.lastSince, want)
	}
}

func TestQuotaChecker_DailyCountErrorFailClosed(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, DailyQuota: 5,
	})
	audits := &mockFederationAuditRepo{
		countFn: func(context.Context, string, string, time.Time) (int, error) {
			return 0, errors.New("db down")
		},
	}
	q := NewQuotaChecker(e, audits, nil, loggateway.NewNoop())
	err := q.Check(context.Background(), FederationLocalOrgID, "org-b")
	if err == nil {
		t.Fatal("Check() expected error (fail-closed on count failure), got nil")
	}
	if apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Error("count failure mapped to 429, want 500-class error (not a quota rejection)")
	}
}

func TestQuotaChecker_LimiterErrorFailClosed(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 10,
	})
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, func(maxPerMin int) Limiter {
		return stubLimiter{allowFn: func(context.Context, string, string) (bool, error) {
			return false, errors.New("redis unreachable")
		}}
	}, loggateway.NewNoop())
	err := q.Check(context.Background(), FederationLocalOrgID, "org-b")
	if err == nil {
		t.Fatal("Check() expected error (fail-closed on limiter failure), got nil")
	}
}

func TestQuotaChecker_NilFactoryFailClosed(t *testing.T) {
	e := newEngineWithPolicy(t, FederationPolicy{
		ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 10,
	})
	q := NewQuotaChecker(e, &mockFederationAuditRepo{}, nil, loggateway.NewNoop())
	err := q.Check(context.Background(), FederationLocalOrgID, "org-b")
	if err == nil {
		t.Fatal("Check() expected error (nil factory with MaxPerMin policy), got nil")
	}
	if apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Error("nil factory mapped to 429, want 500-class misconfiguration error")
	}
}
