package graph

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestCircuitBreakerOptions_PreNodeBlocksWhenOpen(t *testing.T) {
	scope := "team:test-cb-" + t.Name()
	cfg := biz.GraphBuildConfig{
		CircuitBreaker: &biz.CircuitBreakerPolicy{
			FailureThreshold:    1,
			ResetTimeoutSeconds: 60,
		},
		CircuitBreakerScope: scope,
	}
	n := NodeDef{NodeDef: biz.NodeDef{ID: "member-1", Type: biz.NodeTypeAgent}}
	opts := circuitBreakerOptions(n, cfg, nil)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (pre+post), got %d", len(opts))
	}

	cb := biz.DefaultNodeCircuitBreakers().ForNode(scope, n.ID, cfg.CircuitBreaker)
	if cb == nil {
		t.Fatal("expected breaker")
	}
	cb.RecordFailure()
	allowed, st := cb.Allow()
	if allowed {
		t.Fatalf("breaker should be open, state=%s", st)
	}
	msg := biz.CircuitOpenErrorMessage(n.ID, string(st))
	if !strings.Contains(strings.ToLower(msg), "circuit breaker") {
		t.Fatalf("unexpected message: %s", msg)
	}
	if !biz.IsCircuitOpenError(errString(msg)) {
		t.Fatal("IsCircuitOpenError should match")
	}
}

func TestCircuitBreakerOptions_SkippedForRouter(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		CircuitBreaker: &biz.CircuitBreakerPolicy{FailureThreshold: 2},
	}
	n := NodeDef{NodeDef: biz.NodeDef{ID: "r1", Type: biz.NodeTypeRouter}}
	if opts := circuitBreakerOptions(n, cfg, nil); len(opts) != 0 {
		t.Fatalf("router should not get CB options, got %d", len(opts))
	}
}

type errString string

func (e errString) Error() string { return string(e) }
