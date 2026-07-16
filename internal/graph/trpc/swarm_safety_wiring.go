package graph

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// swarmSafetyOptions enforces MaxHandoffs + repetitive-handoff detection on the
// Graph path (native team.Option SwarmSafetyOptions cannot apply to GraphAgent).
func swarmSafetyOptions(n NodeDef, cfg GraphBuildConfig) []trpcgraph.Option {
	spec := cfg.SwarmSafety
	if spec == nil {
		return nil
	}
	if spec.MaxHandoffs <= 0 && (spec.RepetitiveHandoffWindow <= 0 || spec.RepetitiveHandoffMinUnique <= 0) {
		return nil
	}
	nt := normalizeNodeType(n.Type)
	if nt != biz.NodeTypeAgent {
		return nil
	}
	entry := strings.TrimSpace(cfg.EntryPoint)
	nodeID := n.ID
	maxHandoffs := spec.MaxHandoffs
	window := spec.RepetitiveHandoffWindow
	minUnique := spec.RepetitiveHandoffMinUnique

	return []trpcgraph.Option{
		trpcgraph.WithPreNodeCallback(func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, state trpcgraph.State) (any, error) {
			delta, err := enforceSwarmSafety(state, nodeID, entry, maxHandoffs, window, minUnique)
			if err != nil {
				return nil, err
			}
			return delta, nil
		}),
	}
}

// enforceSwarmSafety returns a state delta for handoff tracking, or an error when limits are hit.
func enforceSwarmSafety(state trpcgraph.State, nodeID, entry string, maxHandoffs, window, minUnique int) (trpcgraph.State, error) {
	if entry != "" && nodeID == entry {
		return nil, nil
	}
	count := swarmHandoffCount(state) + 1
	if maxHandoffs > 0 && count > maxHandoffs {
		return nil, apierror.BadRequest(apierror.DomainTeam, fmt.Sprintf("max handoffs exceeded: %d", maxHandoffs))
	}
	recent := append(swarmRecentTargets(state), nodeID)
	if window > 0 && minUnique > 0 {
		if len(recent) > window {
			recent = recent[len(recent)-window:]
		}
		if len(recent) == window && uniqueStringCount(recent) < minUnique {
			return nil, apierror.BadRequest(apierror.DomainTeam, "repetitive handoff detected")
		}
	}
	return trpcgraph.State{
		biz.SwarmHandoffCountStateKey:  count,
		biz.SwarmRecentTargetsStateKey: recent,
	}, nil
}

func swarmHandoffCount(state trpcgraph.State) int {
	if state == nil {
		return 0
	}
	switch v := state[biz.SwarmHandoffCountStateKey].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func swarmRecentTargets(state trpcgraph.State) []string {
	if state == nil {
		return nil
	}
	switch v := state[biz.SwarmRecentTargetsStateKey].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func uniqueStringCount(items []string) int {
	seen := make(map[string]struct{}, len(items))
	for _, s := range items {
		seen[s] = struct{}{}
	}
	return len(seen)
}
