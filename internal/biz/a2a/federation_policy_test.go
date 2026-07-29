package a2a

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type mockFederationPolicyRepo struct {
	listFn   func(context.Context) ([]FederationPolicy, error)
	upsertFn func(context.Context, FederationPolicy) (FederationPolicy, error)
	getFn    func(context.Context, string, string) (FederationPolicy, error)
	deleteFn func(context.Context, string) error
}

func (m *mockFederationPolicyRepo) ListPolicies(ctx context.Context) ([]FederationPolicy, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockFederationPolicyRepo) UpsertPolicy(ctx context.Context, p FederationPolicy) (FederationPolicy, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, p)
	}
	return p, nil
}

func (m *mockFederationPolicyRepo) GetPolicy(ctx context.Context, caller, callee string) (FederationPolicy, error) {
	if m.getFn != nil {
		return m.getFn(ctx, caller, callee)
	}
	return FederationPolicy{}, nil
}

func (m *mockFederationPolicyRepo) DeletePolicy(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func TestPolicyEngine_LoadAndEvaluate(t *testing.T) {
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{
				{ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionDeny},
				{ID: "p2", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-c", Action: PolicyActionAllow, MaxPerMin: 10, DailyQuota: 100},
			}, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	p, found := e.Evaluate(FederationLocalOrgID, "org-b")
	if !found {
		t.Fatal("Evaluate(local, org-b) not found, want found")
	}
	if p.Action != PolicyActionDeny {
		t.Errorf("Action = %q, want %q", p.Action, PolicyActionDeny)
	}

	p, found = e.Evaluate(FederationLocalOrgID, "org-c")
	if !found {
		t.Fatal("Evaluate(local, org-c) not found, want found")
	}
	if p.MaxPerMin != 10 || p.DailyQuota != 100 {
		t.Errorf("quotas = (%d, %d), want (10, 100)", p.MaxPerMin, p.DailyQuota)
	}

	if _, found = e.Evaluate(FederationLocalOrgID, "org-unknown"); found {
		t.Error("Evaluate(local, org-unknown) found, want not found")
	}
	// Key must be an exact ordered pair: reversed direction must miss.
	if _, found = e.Evaluate("org-b", FederationLocalOrgID); found {
		t.Error("Evaluate(org-b, local) found, want not found (directional pair)")
	}
}

func TestPolicyEngine_LoadError(t *testing.T) {
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return nil, errors.New("db down")
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err == nil {
		t.Fatal("Load() expected error, got nil")
	}
}

func TestPolicyEngine_IsDenyAction(t *testing.T) {
	e := NewPolicyEngine(&mockFederationPolicyRepo{}, loggateway.NewNoop())
	tests := []struct {
		action string
		want   bool
	}{
		{PolicyActionDeny, true},
		{PolicyActionApproval, true}, // treated as deny this iteration (design F.5)
		{PolicyActionAllow, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := e.IsDenyAction(tt.action); got != tt.want {
			t.Errorf("IsDenyAction(%q) = %v, want %v", tt.action, got, tt.want)
		}
	}
}

func TestPolicyEngine_UpsertWritesThroughCache(t *testing.T) {
	var stored []FederationPolicy
	repo := &mockFederationPolicyRepo{
		upsertFn: func(_ context.Context, p FederationPolicy) (FederationPolicy, error) {
			p.ID = "p-new"
			stored = append(stored, p)
			return p, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Insert new pair: immediately visible without reload.
	if _, err := e.UpsertPolicy(context.Background(), FederationPolicy{
		CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-d", Action: PolicyActionDeny,
	}); err != nil {
		t.Fatalf("UpsertPolicy() error: %v", err)
	}
	p, found := e.Evaluate(FederationLocalOrgID, "org-d")
	if !found || p.Action != PolicyActionDeny {
		t.Fatalf("Evaluate after upsert = (%+v, %v), want deny policy found", p, found)
	}

	// Update same pair: cache reflects the new action/quotas.
	if _, err := e.UpsertPolicy(context.Background(), FederationPolicy{
		CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-d", Action: PolicyActionAllow, MaxPerMin: 5,
	}); err != nil {
		t.Fatalf("UpsertPolicy() update error: %v", err)
	}
	p, found = e.Evaluate(FederationLocalOrgID, "org-d")
	if !found || p.Action != PolicyActionAllow || p.MaxPerMin != 5 {
		t.Fatalf("Evaluate after update = (%+v, %v), want allow policy MaxPerMin=5", p, found)
	}
	if len(stored) != 2 {
		t.Errorf("repo upsert calls = %d, want 2", len(stored))
	}
}

func TestPolicyEngine_DeleteInvalidatesCache(t *testing.T) {
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{
				{ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionDeny},
			}, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if _, found := e.Evaluate(FederationLocalOrgID, "org-b"); !found {
		t.Fatal("precondition: policy cached")
	}
	if err := e.DeletePolicy(context.Background(), "p1"); err != nil {
		t.Fatalf("DeletePolicy() error: %v", err)
	}
	if _, found := e.Evaluate(FederationLocalOrgID, "org-b"); found {
		t.Error("Evaluate after delete found, want invalidated")
	}
}

func TestPolicyEngine_ListPoliciesFromCache(t *testing.T) {
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{
				{ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionAllow},
				{ID: "p2", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-c", Action: PolicyActionDeny},
			}, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	got := e.ListPolicies()
	if len(got) != 2 {
		t.Fatalf("ListPolicies() len = %d, want 2", len(got))
	}
}

func TestPolicyEngine_ConcurrentAccess(t *testing.T) {
	repo := &mockFederationPolicyRepo{
		listFn: func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{
				{ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionAllow},
			}, nil
		},
	}
	e := NewPolicyEngine(repo, loggateway.NewNoop())
	if err := e.Load(context.Background()); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = e.Evaluate(FederationLocalOrgID, "org-b")
		}()
		go func() {
			defer wg.Done()
			_, _ = e.UpsertPolicy(context.Background(), FederationPolicy{
				CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionAllow,
			})
		}()
	}
	wg.Wait()
}
