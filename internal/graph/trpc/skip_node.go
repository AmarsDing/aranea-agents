package graph

import (
	"context"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// SkipNodeFunc returns a node function that records the node as skipped in graph state.
func SkipNodeFunc(nodeID string) trpcgraph.NodeFunc {
	id := nodeID
	return func(_ context.Context, state trpcgraph.State) (any, error) {
		if state == nil {
			state = trpcgraph.State{}
		}
		skipped := appendSkippedNode(state[biz.SkippedNodesStateKey], id)
		state[biz.SkippedNodesStateKey] = skipped
		return map[string]any{biz.SkippedNodeOutputKey: id}, nil
	}
}

func appendSkippedNode(raw any, nodeID string) []string {
	out := make([]string, 0, 4)
	switch v := raw.(type) {
	case []string:
		out = append(out, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	for _, existing := range out {
		if existing == nodeID {
			return out
		}
	}
	return append(out, nodeID)
}
