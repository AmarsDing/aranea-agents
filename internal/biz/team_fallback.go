package biz

// FallbackPolicy controls the team runtime fallback behavior when the primary
// execution engine (Graph) is unavailable or fails. This decouples the fallback
// decision from the team runner implementation.
type FallbackPolicy struct {
	// Enabled controls whether native team runtime is available as a fallback.
	Enabled bool

	// Reason describes why fallback is enabled/disabled (for logging and metrics).
	Reason string

	// CanaryPercentage is the percentage of team turns that should use the
	// primary engine even when fallback is enabled (for gradual rollout).
	// 0 = all fallback, 100 = all primary.
	CanaryPercentage int
}

// DefaultFallbackPolicy returns the production default: Graph-only, no native fallback.
func DefaultFallbackPolicy() FallbackPolicy {
	return FallbackPolicy{
		Enabled:          false,
		Reason:           "graph-only production default",
		CanaryPercentage: 100,
	}
}

// NativeFallbackPolicy returns a policy that enables native runtime fallback.
func NativeFallbackPolicy() FallbackPolicy {
	return FallbackPolicy{
		Enabled:          true,
		Reason:           "native fallback enabled via config",
		CanaryPercentage: 0,
	}
}

// FailurePolicyConfig is the biz-level representation of team failure policy,
// used in TeamDefinition to avoid importing internal/team concrete types.
type FailurePolicyConfig = TeamFailurePolicy
