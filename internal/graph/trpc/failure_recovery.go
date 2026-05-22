package graph

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func failureRecoveryOptions(n NodeDef) []trpcgraph.Option {
	if strings.EqualFold(strings.TrimSpace(n.FuncRef), biz.SkipNodeFuncRef) {
		return nil
	}
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	if action != biz.FailureOnFailureSkip && fallback == "" {
		return nil
	}
	return []trpcgraph.Option{trpcgraph.WithPostNodeCallback(failureRecoveryAfterNode(n))}
}

func failureRecoveryAfterNode(n NodeDef) trpcgraph.AfterNodeCallback {
	nodeID := n.ID
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	primary := strings.TrimSpace(n.AgentName)
	return func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, state trpcgraph.State, _ any, nodeErr error) (any, error) {
		if nodeErr == nil {
			return nil, nil
		}
		if fallback != "" {
			fn := trpcgraph.NewAgentNodeFunc(fallback)
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

func skipNodeUpdate(state trpcgraph.State, nodeID string) map[string]any {
	if state == nil {
		state = trpcgraph.State{}
	}
	skipped := appendSkippedNode(state[biz.SkippedNodesStateKey], nodeID)
	state[biz.SkippedNodesStateKey] = skipped
	return map[string]any{biz.SkippedNodeOutputKey: nodeID}
}
