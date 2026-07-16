package graph

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// circuitBreakerOptions wires Pre/Post node callbacks for FP-02 consecutive-failure
// circuit breaking. PreNode rejects execution when the breaker is open; PostNode
// records success/failure after the node's final attempt (framework semantics).
func circuitBreakerOptions(n NodeDef, cfg GraphBuildConfig, reg *biz.NodeCircuitBreakerRegistry) []trpcgraph.Option {
	pol := cfg.CircuitBreaker
	if pol == nil || pol.FailureThreshold <= 0 {
		return nil
	}
	nt := normalizeNodeType(n.Type)
	if nt != biz.NodeTypeAgent && nt != biz.NodeTypeLLM && nt != biz.NodeTypeTool && nt != biz.NodeTypeTools {
		return nil
	}
	if reg == nil {
		reg = biz.DefaultNodeCircuitBreakers()
	}
	cb := reg.ForNode(cfg.CircuitBreakerScope, n.ID, pol)
	if cb == nil {
		return nil
	}
	nodeID := n.ID
	return []trpcgraph.Option{
		trpcgraph.WithPreNodeCallback(func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, _ trpcgraph.State) (any, error) {
			allowed, st := cb.Allow()
			if allowed {
				return nil, nil
			}
			return nil, fmt.Errorf("%s", biz.CircuitOpenErrorMessage(nodeID, string(st)))
		}),
		trpcgraph.WithPostNodeCallback(func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, _ trpcgraph.State, _ any, nodeErr error) (any, error) {
			if nodeErr == nil {
				cb.RecordSuccess()
				return nil, nil
			}
			// Do not count circuit-open rejections as additional failures.
			if biz.IsCircuitOpenError(nodeErr) {
				return nil, nil
			}
			prev := cb.State()
			cb.RecordFailure()
			if prev != biztool.CircuitOpen && cb.State() == biztool.CircuitOpen {
				// Error message is already on the node failure path; returning nil
				// lets failure_recovery / executor keep the original nodeErr.
				_ = strings.TrimSpace(nodeID)
			}
			return nil, nil
		}),
	}
}
