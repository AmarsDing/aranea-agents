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

type CircuitBreakerState struct {
	mu       sync.Mutex
	breakers map[string]*nodeBreaker
}

func NewCircuitBreakerState() *CircuitBreakerState {
	return &CircuitBreakerState{
		breakers: map[string]*nodeBreaker{},
	}
}

func (s *CircuitBreakerState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breakers = map[string]*nodeBreaker{}
}

func (s *CircuitBreakerState) State(nodeID string) breakerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.breakers[nodeID]; b != nil {
		return b.state
	}
	return breakerClosed
}

func circuitBreakerOptions(n NodeDef, policy *biz.CircuitBreakerPolicy, cbState *CircuitBreakerState) []trpcgraph.Option {
	if policy == nil || policy.FailureThreshold <= 0 || cbState == nil {
		return nil
	}
	nodeID := n.ID
	threshold := policy.FailureThreshold
	return []trpcgraph.Option{trpcgraph.WithPostNodeCallback(cbState.afterNode(nodeID, threshold))}
}

func (s *CircuitBreakerState) afterNode(nodeID string, threshold int) trpcgraph.AfterNodeCallback {
	return func(_ context.Context, _ *trpcgraph.NodeCallbackContext, _ trpcgraph.State, _ any, nodeErr error) (any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		b := s.breakers[nodeID]
		if b == nil {
			b = &nodeBreaker{state: breakerClosed, threshold: threshold}
			s.breakers[nodeID] = b
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
