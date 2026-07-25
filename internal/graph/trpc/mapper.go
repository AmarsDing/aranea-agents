package graph

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"

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

// agentOutputMapperFor resolves the output mapper for a node. A user-provided
// output_mapper_json always wins — explicit mapping is a deliberate contract.
// Agent nodes in a deliverable-enabled graph (EnableStateDeliverable →
// deliverable StateField) fall back to the deliverable-aware mapper so
// set_deliverable writes actually land in graph state; all other nodes keep
// the framework default.
func agentOutputMapperFor(n NodeDef, cfg GraphBuildConfig) trpcgraph.SubgraphOutputMapper {
	if m := parseOutputMapper(n.OutputMapperJSON); m != nil {
		return m
	}
	if normalizeNodeType(n.Type) != biz.NodeTypeAgent {
		return nil
	}
	if !cfgHasDeliverableStateField(cfg) {
		return nil
	}
	return deliverableAwareOutputMapper(n.ID)
}

// cfgHasDeliverableStateField reports whether the graph schema carries the
// deliverable StateField injected by EnableStateDeliverable
// (team.finalizeRuntimeGraphConfig → ensureDeliverableStateField).
func cfgHasDeliverableStateField(cfg GraphBuildConfig) bool {
	for _, sf := range cfg.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			return true
		}
	}
	return false
}

// deliverableAwareOutputMapper mirrors the framework's default agent-node
// output mapping (last_response + node_responses) and additionally merges the
// deliverable entry from the node result's effective state delta. Plain agent
// nodes capture tool-event state deltas (set_deliverable) only into the
// fallback delta, which the framework default mapping drops on success —
// without this, member-to-member deliverable handoff never reaches graph
// state. Only the deliverable key is merged; unrelated delta keys stay out of
// the node output so other tools' state deltas keep their existing semantics.
func deliverableAwareOutputMapper(nodeID string) trpcgraph.SubgraphOutputMapper {
	return func(_ trpcgraph.State, r trpcgraph.SubgraphResult) trpcgraph.State {
		upd := trpcgraph.State{}
		upd[trpcgraph.StateKeyLastResponse] = r.LastResponse
		upd[trpcgraph.StateKeyNodeResponses] = map[string]any{
			nodeID: r.LastResponse,
		}
		if delta := r.EffectiveStateDelta(); len(delta) > 0 {
			if raw, ok := delta[biz.DeliverableStateKey]; ok && len(raw) > 0 {
				var deliverable map[string]any
				if err := json.Unmarshal(raw, &deliverable); err == nil {
					upd[biz.DeliverableStateKey] = deliverable
				}
			}
		}
		return upd
	}
}
