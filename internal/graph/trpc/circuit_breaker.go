package graph

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

type nodeBreaker struct {
	state            breakerState
	consecutiveFails int
	threshold        int
}

var (
	breakerMu sync.Mutex
	breakers  = map[string]*nodeBreaker{}
)

func circuitBreakerOptions(n NodeDef, policy *biz.CircuitBreakerPolicy) []trpcgraph.Option {
	if policy == nil || policy.FailureThreshold <= 0 {
		return nil
	}
	nodeID := n.ID
	threshold := policy.FailureThreshold
	return []trpcgraph.Option{trpcgraph.WithPostNodeCallback(circuitBreakerAfterNode(nodeID, threshold))}
}

func circuitBreakerAfterNode(nodeID string, threshold int) trpcgraph.AfterNodeCallback {
	return func(_ context.Context, _ *trpcgraph.NodeCallbackContext, _ trpcgraph.State, _ any, nodeErr error) (any, error) {
		breakerMu.Lock()
		defer breakerMu.Unlock()
		b := breakers[nodeID]
		if b == nil {
			b = &nodeBreaker{state: breakerClosed, threshold: threshold}
			breakers[nodeID] = b
		}
		if nodeErr != nil {
			b.consecutiveFails++
			if b.consecutiveFails >= b.threshold {
				b.state = breakerOpen
			}
			return nil, nodeErr
		}
		switch b.state {
		case breakerOpen:
			b.state = breakerHalfOpen
			b.consecutiveFails = 0
		case breakerHalfOpen:
			b.state = breakerClosed
			b.consecutiveFails = 0
		default:
			b.consecutiveFails = 0
			b.state = breakerClosed
		}
		return nil, nil
	}
}

// ResetCircuitBreakers clears in-memory breaker state (tests).
func ResetCircuitBreakers() {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	breakers = map[string]*nodeBreaker{}
}

// CircuitBreakerState exposes state for tests.
func CircuitBreakerState(nodeID string) breakerState {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	if b := breakers[nodeID]; b != nil {
		return b.state
	}
	return breakerClosed
}
