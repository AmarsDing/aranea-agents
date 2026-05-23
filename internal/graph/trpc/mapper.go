package graph

import (
	"encoding/json"
	"strings"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func parseInputMapper(jsonStr string) trpcgraph.SubgraphInputMapper {
	mapping := parseFieldMapping(jsonStr)
	if len(mapping) == 0 {
		return nil
	}
	return func(parent trpcgraph.State) trpcgraph.State {
		child := trpcgraph.State{}
		for src, dst := range mapping {
			if v, ok := parent[src]; ok {
				child[dst] = v
			}
		}
		return child
	}
}

func parseOutputMapper(jsonStr string) trpcgraph.SubgraphOutputMapper {
	mapping := parseFieldMapping(jsonStr)
	if len(mapping) == 0 {
		return nil
	}
	return func(_ trpcgraph.State, r trpcgraph.SubgraphResult) trpcgraph.State {
		out := trpcgraph.State{}
		state := r.EffectiveState()
		if state == nil {
			state = trpcgraph.State{}
		}
		for src, dst := range mapping {
			if v, ok := state[src]; ok {
				out[dst] = v
			}
		}
		return out
	}
}

func parseFieldMapping(jsonStr string) map[string]string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &mapping); err != nil {
		return nil
	}
	if len(mapping) == 0 {
		return nil
	}
	return mapping
}
