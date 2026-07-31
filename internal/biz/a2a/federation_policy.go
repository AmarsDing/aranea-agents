package a2a

import (
	"context"
	"sync"

	"aranea-agents/pkg/loggateway"
)

// policyCacheKey builds the exact ordered-pair cache key. The NUL separator
// cannot appear in org IDs (validated as hostnames / "local").
func policyCacheKey(callerOrgID, calleeOrgID string) string {
	return callerOrgID + "\x00" + calleeOrgID
}

// normalizeAction maps reserved actions to their effective behavior without
// logging: approval is treated as deny this iteration (design F.5/F.1#7).
func normalizeAction(action string) string {
	if action == PolicyActionApproval {
		return PolicyActionDeny
	}
	return action
}

// PolicyEngine evaluates org-pair call policies from an in-memory cache
// (design F.5): the full table is loaded at startup via Load; Upsert/Delete
// write through to the repo and refresh the cache synchronously. Evaluation
// order: explicit policy (exact caller+callee pair) > trust-level default
// (handled by the caller when Evaluate reports not-found).
type PolicyEngine struct {
	mu    sync.RWMutex
	repo  FederationPolicyRepo
	lg    loggateway.Logger
	cache map[string]FederationPolicy
}

// NewPolicyEngine constructs a PolicyEngine. Load must be called once at
// startup before the engine serves traffic.
func NewPolicyEngine(repo FederationPolicyRepo, lg loggateway.Logger) *PolicyEngine {
	return &PolicyEngine{
		repo:  repo,
		lg:    lg,
		cache: make(map[string]FederationPolicy),
	}
}

// Load replaces the cache with the full policy table.
func (e *PolicyEngine) Load(ctx context.Context) error {
	policies, err := e.repo.ListPolicies(ctx)
	if err != nil {
		return err
	}
	fresh := make(map[string]FederationPolicy, len(policies))
	for _, p := range policies {
		fresh[policyCacheKey(p.CallerOrgID, p.CalleeOrgID)] = p
	}
	e.mu.Lock()
	e.cache = fresh
	e.mu.Unlock()
	return nil
}

// Evaluate returns the explicit policy for the ordered org pair.
// found=false means no explicit policy — the caller falls back to the
// trust-level default (allow, no quota).
func (e *PolicyEngine) Evaluate(callerOrgID, calleeOrgID string) (FederationPolicy, bool) {
	e.mu.RLock()
	p, found := e.cache[policyCacheKey(callerOrgID, calleeOrgID)]
	e.mu.RUnlock()
	// K5: explicit deny policies always block the call (approval normalizes to
	// deny and is Warned in IsDenyAction); log the denial rule hit once here.
	if found && p.Action == PolicyActionDeny && e != nil && e.lg != nil {
		e.lg.Warn("federation call denied by policy rule",
			loggateway.StepID("a2a.fed.policy.denied"),
			loggateway.Str("policy_id", p.ID),
			loggateway.Str("caller_org_id", callerOrgID),
			loggateway.Str("callee_org_id", calleeOrgID),
			loggateway.Str("action", p.Action),
		)
	}
	return p, found
}

// IsDenyAction reports whether a policy action blocks the call. approval is
// reserved and treated as deny this iteration; the Warn is logged here (once
// per evaluation) rather than in normalizeAction so hot paths that only need
// the effective action stay quiet.
func (e *PolicyEngine) IsDenyAction(action string) bool {
	if action == PolicyActionApproval && e != nil && e.lg != nil {
		e.lg.Warn("federation policy action=approval treated as deny; no approval workflow this iteration",
			loggateway.StepID("a2a.fed.policy.evaluate"),
		)
	}
	return normalizeAction(action) == PolicyActionDeny
}

// UpsertPolicy persists the policy and refreshes the cache entry.
func (e *PolicyEngine) UpsertPolicy(ctx context.Context, p FederationPolicy) (FederationPolicy, error) {
	stored, err := e.repo.UpsertPolicy(ctx, p)
	if err != nil {
		return FederationPolicy{}, err
	}
	e.mu.Lock()
	e.cache[policyCacheKey(stored.CallerOrgID, stored.CalleeOrgID)] = stored
	e.mu.Unlock()
	return stored, nil
}

// DeletePolicy removes the policy by ID and invalidates its cache entry.
func (e *PolicyEngine) DeletePolicy(ctx context.Context, id string) error {
	if err := e.repo.DeletePolicy(ctx, id); err != nil {
		return err
	}
	e.mu.Lock()
	for k, p := range e.cache {
		if p.ID == id {
			delete(e.cache, k)
			break
		}
	}
	e.mu.Unlock()
	return nil
}

// ListPolicies returns a snapshot of the cached policies.
func (e *PolicyEngine) ListPolicies() []FederationPolicy {
	e.mu.RLock()
	out := make([]FederationPolicy, 0, len(e.cache))
	for _, p := range e.cache {
		out = append(out, p)
	}
	e.mu.RUnlock()
	return out
}
