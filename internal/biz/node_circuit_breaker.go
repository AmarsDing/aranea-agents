package biz

import (
	"context"
	"fmt"
	"strings"
	"sync"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"
)

// NodeCircuitBreakerRegistry provides process-scoped circuit breakers for
// graph agent/llm/tool nodes (M53 FP-02). Keys are namespaced by
// CircuitBreakerScope so consecutive failures persist across Team Runs.
type NodeCircuitBreakerRegistry struct {
	mu    sync.Mutex
	inner *biztool.CircuitBreakerRegistry
}

var (
	defaultNodeCircuitBreakersMu sync.RWMutex
	defaultNodeCircuitBreakers   = NewNodeCircuitBreakerRegistry()
)

// DefaultNodeCircuitBreakers returns the process-wide registry used by graph runtime.
// Prefer injecting a registry via GraphNodeResolverSet.NodeBreakers in production.
func DefaultNodeCircuitBreakers() *NodeCircuitBreakerRegistry {
	defaultNodeCircuitBreakersMu.RLock()
	defer defaultNodeCircuitBreakersMu.RUnlock()
	return defaultNodeCircuitBreakers
}

// SetDefaultNodeCircuitBreakers replaces the process default (used by Wire / tests).
func SetDefaultNodeCircuitBreakers(reg *NodeCircuitBreakerRegistry) {
	if reg == nil {
		return
	}
	defaultNodeCircuitBreakersMu.Lock()
	defaultNodeCircuitBreakers = reg
	defaultNodeCircuitBreakersMu.Unlock()
}

// NewNodeCircuitBreakerRegistry constructs a node breaker registry.
func NewNodeCircuitBreakerRegistry(opts ...biztool.CircuitBreakerRegistryOption) *NodeCircuitBreakerRegistry {
	return &NodeCircuitBreakerRegistry{
		inner: biztool.NewCircuitBreakerRegistry(opts...),
	}
}

// ProvideNodeCircuitBreakerRegistry builds a persisted registry and restores state.
func ProvideNodeCircuitBreakerRegistry(repo biztool.CircuitBreakerStateRepo, lg loggateway.Logger) *NodeCircuitBreakerRegistry {
	opts := []biztool.CircuitBreakerRegistryOption{}
	if repo != nil {
		opts = append(opts, biztool.WithStateRepo(repo))
	}
	if lg != nil {
		opts = append(opts, biztool.WithLogger(lg))
	}
	reg := NewNodeCircuitBreakerRegistry(opts...)
	reg.RestoreStates(context.Background())
	SetDefaultNodeCircuitBreakers(reg)
	return reg
}

// RestoreStates reloads persisted breaker snapshots after process restart.
func (r *NodeCircuitBreakerRegistry) RestoreStates(ctx context.Context) {
	if r == nil || r.inner == nil {
		return
	}
	r.inner.RestoreStates(ctx)
}

func (r *NodeCircuitBreakerRegistry) key(scope, nodeID string) string {
	scope = strings.TrimSpace(scope)
	nodeID = strings.TrimSpace(nodeID)
	if scope == "" {
		scope = "graph"
	}
	return fmt.Sprintf("graph_node:%s:node:%s", scope, nodeID)
}

// ForNode returns (or creates) a breaker for the given node under scope.
func (r *NodeCircuitBreakerRegistry) ForNode(scope, nodeID string, pol *CircuitBreakerPolicy) *biztool.CircuitBreaker {
	if r == nil || pol == nil || pol.FailureThreshold <= 0 || strings.TrimSpace(nodeID) == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := r.key(scope, nodeID)
	cfg := biztool.CircuitBreakerConfig{
		FailureThreshold:   pol.FailureThreshold,
		RecoveryTimeoutSec: pol.ResetTimeoutSeconds,
		HalfOpenMaxProbe:   1,
	}
	r.inner.SetOverride(name, cfg)
	return r.inner.Get(name, "")
}

// IsCircuitOpenError reports whether err indicates a circuit-open rejection.
func IsCircuitOpenError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "circuit breaker open") || strings.Contains(msg, "circuit breaker opened")
}

// CircuitOpenErrorMessage builds a stable error string for PreNode rejection /
// Observability banner heuristics.
func CircuitOpenErrorMessage(nodeID string, state string) string {
	nodeID = strings.TrimSpace(nodeID)
	state = strings.TrimSpace(state)
	if state == "" {
		state = string(biztool.CircuitOpen)
	}
	return fmt.Sprintf("circuit breaker open for node %s (state=%s)", nodeID, state)
}
