// Package memory provides the spreading-activation retrieval engine for the
// L4 entity graph. It implements the weighted propagation algorithm described
// in docs/development/70-orchestration-longtask-memory.design.md §15.5.
package memory

import (
	"context"
	"math"
	"sort"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// SpreadingActivationEngine performs weighted spreading activation over the L4
// entity graph. It depends on the narrow biz.L4GraphTraverser interface so
// that mock implementations only need to stub GraphTraverseCTE.
//
// Algorithm (per design §15.5):
//  1. Recursive CTE fetches the subgraph within `hops` levels.
//  2. Center node starts with activation = 1.0.
//  3. Each hop propagates: activation[neighbor] += activation[current] * weight * decayFactor(hop)
//  4. Top-K pruning keeps only the K highest-activation nodes per hop.
//  5. Decay factor: 0.7^(hop-1) — hop1=1.0, hop2=0.7, hop3=0.49.
//  6. Threshold: activation < 0.01 is not propagated.
//  7. INHIBIT edges do not propagate activation (RelationTypeProp.InhibitsTarget).
type SpreadingActivationEngine struct {
	traverser biz.L4GraphTraverser
	lg        loggateway.Logger
}

// NewSpreadingActivationEngine constructs a SpreadingActivationEngine. The
// traverser must be non-nil; lg may be a noop logger for tests.
func NewSpreadingActivationEngine(traverser biz.L4GraphTraverser, lg loggateway.Logger) *SpreadingActivationEngine {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SpreadingActivationEngine{traverser: traverser, lg: lg}
}

// SpreadingActivation executes a spreading-activation query from centerID.
//
// centerID: the seed neuron ID (activation = 1.0).
// hops: maximum propagation depth (default 3 if <= 0).
// topK: maximum number of returned results including the center node
// (default 20 if <= 0). Per-hop frontier pruning uses topK to bound
// propagation breadth; a final pruning step trims the result list to topK
// so callers get a predictable result size.
//
// Returns results sorted by activation descending. The center node is always
// included with activation = 1.0 and hop = 0.
func (e *SpreadingActivationEngine) SpreadingActivation(
	ctx context.Context, centerID string, hops, topK int,
) ([]biz.L4ActivationResult, error) {
	if e == nil || e.traverser == nil {
		return nil, nil
	}
	if hops <= 0 {
		hops = 3
	}
	if topK <= 0 {
		topK = 20
	}

	// 1. Fetch the subgraph via recursive CTE. We request topK*2 nodes to give
	//    the application-layer propagation some headroom before pruning.
	graph, err := e.traverser.GraphTraverseCTE(ctx, centerID, hops, topK*2)
	if err != nil {
		e.lg.Warn("spreading activation: GraphTraverseCTE failed",
			loggateway.Err(err))
		return nil, err
	}
	if graph == nil || len(graph.Nodes) == 0 {
		return nil, nil
	}

	// 2. Initialize: center node activation = 1.0.
	// maxActivations tracks the strongest activation each node ever received
	// across all hops (so a node activated strongly in hop 1 but weakly in
	// hop 3 retains the hop-1 value).
	maxActivations := map[string]float64{centerID: 1.0}
	hopCounts := map[string]int{centerID: 0}
	paths := map[string][]biz.L4PathStep{}
	// currentActivations is the frontier for the current hop (post-pruning).
	currentActivations := map[string]float64{centerID: 1.0}

	// 3. Per-hop propagation.
	for hop := 1; hop <= hops; hop++ {
		nextActivations := map[string]float64{}
		for nodeID, activation := range currentActivations {
			if activation < activationThreshold {
				continue
			}
			neighbors := graph.Neighbors(nodeID)
			for _, edge := range neighbors {
				// INHIBIT edges do not propagate positive activation.
				if prop, ok := biz.LookupRelationTypeProp(edge.RelationType); ok && prop.InhibitsTarget {
					continue
				}
				target := edge.TargetID
				if target == nodeID {
					target = edge.SourceID
				}
				if target == centerID {
					continue // do not back-propagate to center
				}
				propagated := activation * edge.Weight * decayFactor(hop)
				if propagated < activationThreshold {
					continue
				}
				nextActivations[target] += propagated
				// Track the maximum activation across all hops.
				if propagated > maxActivations[target] {
					maxActivations[target] = propagated
				}
				// Keep the shortest hop count.
				if _, exists := hopCounts[target]; !exists {
					hopCounts[target] = hop
				}
				// Record the propagation path (keep the strongest contributor).
				if len(paths[target]) == 0 || propagated > bestPathScore(paths[target]) {
					paths[target] = []biz.L4PathStep{{
						FromNodeID:   nodeID,
						ToNodeID:     target,
						EdgeWeight:   edge.Weight,
						RelationType: edge.RelationType,
					}}
				}
			}
		}
		// 4. Top-K pruning on the current hop's new activations.
		currentActivations = topKFilter(nextActivations, topK)
	}

	// 5. Build results from maxActivations (sorted by activation desc).
	results := make([]biz.L4ActivationResult, 0, len(maxActivations))
	for id, act := range maxActivations {
		results = append(results, biz.L4ActivationResult{
			NodeID:         id,
			Activation:     act,
			HopCount:       hopCounts[id],
			ActivationPath: paths[id],
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Activation > results[j].Activation
	})
	// 6. Final Top-K pruning: trim result list to topK (including center).
	// Per-hop pruning bounds the propagation frontier; this final trim
	// guarantees the caller receives at most topK results.
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// bestPathScore computes the effective activation score of the first (and
// typically only) path step, used to decide whether to replace the recorded
// path with a stronger contributor.
func bestPathScore(path []biz.L4PathStep) float64 {
	if len(path) == 0 {
		return 0
	}
	return path[0].EdgeWeight
}

// activationThreshold is the minimum activation value to propagate (NFR-1.8).
const activationThreshold = 0.01

// decayFactor returns the per-hop decay factor: 0.7^(hop-1).
// hop=1 → 1.0, hop=2 → 0.7, hop=3 → 0.49.
func decayFactor(hop int) float64 {
	return math.Pow(0.7, float64(hop-1))
}

// topKFilter returns a new map containing only the top K entries by value
// (descending). If len(m) <= k, returns m unchanged (as a new map).
func topKFilter(m map[string]float64, k int) map[string]float64 {
	if len(m) <= k {
		out := make(map[string]float64, len(m))
		for id, v := range m {
			out[id] = v
		}
		return out
	}
	type entry struct {
		id string
		v  float64
	}
	entries := make([]entry, 0, len(m))
	for id, v := range m {
		entries = append(entries, entry{id, v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].v > entries[j].v })
	if k > len(entries) {
		k = len(entries)
	}
	out := make(map[string]float64, k)
	for i := 0; i < k; i++ {
		out[entries[i].id] = entries[i].v
	}
	return out
}

// Compile-time interface checks.
var (
	_ biz.L4SpreadingActivationEngine = (*SpreadingActivationEngine)(nil)
)
