package graph

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func failureRecoveryOptions(n NodeDef, resolvedFallback trpcagent.Agent) []trpcgraph.Option {
	if strings.EqualFold(strings.TrimSpace(n.FuncRef), biz.SkipNodeFuncRef) {
		return nil
	}
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	if action != biz.FailureOnFailureSkip && fallback == "" {
		return nil
	}
	return []trpcgraph.Option{trpcgraph.WithPostNodeCallback(failureRecoveryAfterNode(n, resolvedFallback))}
}

func failureRecoveryAfterNode(n NodeDef, resolvedFallback trpcagent.Agent) trpcgraph.AfterNodeCallback {
	nodeID := n.ID
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	primary := strings.TrimSpace(n.AgentName)
	return func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, state trpcgraph.State, _ any, nodeErr error) (any, error) {
		if nodeErr == nil {
			return nil, nil
		}
		if fallback != "" {
			var fn trpcgraph.NodeFunc
			if resolvedFallback != nil {
				fn = resolvedAgentNodeFunc(resolvedFallback)
			} else {
				fn = trpcgraph.NewAgentNodeFunc(fallback)
			}
			out, err := fn(ctx, state)
			if err == nil {
				if st, ok := out.(trpcgraph.State); ok && primary != "" {
					st["_fallback_from_"+nodeID] = primary
					st["_fallback_agent_"+nodeID] = fallback
					return st, nil
				}
				return out, nil
			}
			if action == biz.FailureOnFailureSkip {
				return skipNodeUpdate(state, nodeID), nil
			}
			return nil, err
		}
		if action == biz.FailureOnFailureSkip {
			return skipNodeUpdate(state, nodeID), nil
		}
		return nil, nil
	}
}

func resolvedAgentNodeFunc(ag trpcagent.Agent) trpcgraph.NodeFunc {
	return func(ctx context.Context, state trpcgraph.State) (any, error) {
		parentAgent, _ := state[trpcgraph.StateKeyParentAgent]
		wrapper := &fallbackAgentWrapper{Agent: ag, parent: parentAgent}
		patchedState := make(trpcgraph.State, len(state)+1)
		for k, v := range state {
			patchedState[k] = v
		}
		patchedState[trpcgraph.StateKeyParentAgent] = wrapper
		return trpcgraph.NewAgentNodeFunc(ag.Info().Name)(ctx, patchedState)
	}
}

type fallbackAgentWrapper struct {
	trpcagent.Agent
	parent any
}

func (w *fallbackAgentWrapper) FindSubAgent(name string) trpcagent.Agent {
	if w.Agent != nil && w.Agent.Info().Name == name {
		return w.Agent
	}
	type subAgentProvider interface {
		FindSubAgent(name string) trpcagent.Agent
	}
	if p, ok := w.parent.(subAgentProvider); ok {
		return p.FindSubAgent(name)
	}
	return nil
}

func skipNodeUpdate(state trpcgraph.State, nodeID string) map[string]any {
	if state == nil {
		state = trpcgraph.State{}
	}
	skipped := appendSkippedNode(state[biz.SkippedNodesStateKey], nodeID)
	state[biz.SkippedNodesStateKey] = skipped
	return map[string]any{biz.SkippedNodeOutputKey: nodeID}
}
