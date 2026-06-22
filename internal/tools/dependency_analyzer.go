package tools

import (
	"errors"
	"fmt"

	"aranea-agents/pkg/apierror"
)

// Dependency-analysis sentinel errors. Use errors.Is to check.
var (
	// ErrCycleDetected is returned when Analyze detects a dependency cycle.
	ErrCycleDetected = errors.New("dependency cycle detected")
	// ErrMissingDependency is returned when a call references a non-existent
	// call ID in its DependsOn list.
	ErrMissingDependency = errors.New("missing dependency")
	// ErrDuplicateCallID is returned when two calls share the same ID.
	ErrDuplicateCallID = errors.New("duplicate call id")
)

// DependencyAnalyzer builds a DAG from ToolCalls based on their DependsOn
// relations. The resulting DAG exposes TopologicalLayers for parallel execution.
type DependencyAnalyzer struct{}

// NewDependencyAnalyzer creates a stateless DependencyAnalyzer.
func NewDependencyAnalyzer() *DependencyAnalyzer {
	return &DependencyAnalyzer{}
}

// Analyze validates the input calls and builds a DAG. Returns an error if:
//   - duplicate call IDs are present (ErrDuplicateCallID)
//   - a DependsOn entry references a non-existent ID (ErrMissingDependency)
//   - a cycle is detected (ErrCycleDetected)
func (a *DependencyAnalyzer) Analyze(calls []ToolCall) (*DAG, error) {
	if a == nil {
		return nil, apierror.Internal(apierror.DomainTool, "dependency analyzer not initialized")
	}

	// Build node index and validate uniqueness.
	nodes := make(map[string]*dagNode, len(calls))
	order := make([]string, 0, len(calls))
	for i := range calls {
		c := &calls[i]
		if c.ID == "" {
			return nil, apierror.BadRequest(apierror.DomainTool, "tool call at index %d has empty id", i)
		}
		if _, exists := nodes[c.ID]; exists {
			return nil, fmt.Errorf("%w: id=%s", ErrDuplicateCallID, c.ID)
		}
		nodes[c.ID] = &dagNode{call: c}
		order = append(order, c.ID)
	}

	// Build edges and validate references.
	for i := range calls {
		c := &calls[i]
		for _, dep := range c.DependsOn {
			if dep == c.ID {
				return nil, fmt.Errorf("%w: self-loop on id=%s", ErrCycleDetected, c.ID)
			}
			depNode, ok := nodes[dep]
			if !ok {
				return nil, fmt.Errorf("%w: call %s depends on unknown id=%s", ErrMissingDependency, c.ID, dep)
			}
			depNode.dependents = append(depNode.dependents, c.ID)
			nodes[c.ID].dependencies++
		}
	}

	// Detect cycles via Kahn's algorithm: if we cannot peel all nodes, a cycle exists.
	if hasCycle(nodes) {
		return nil, ErrCycleDetected
	}

	return &DAG{nodes: nodes, order: order}, nil
}

// hasCycle returns true if the graph contains a cycle. Uses Kahn's algorithm:
// repeatedly remove nodes with no remaining dependencies; if any nodes remain
// unprocessed, they form at least one cycle.
func hasCycle(nodes map[string]*dagNode) bool {
	// Work on a copy of dependency counts to avoid mutating the original.
	inDegree := make(map[string]int, len(nodes))
	for id, n := range nodes {
		inDegree[id] = n.dependencies
	}
	queue := make([]string, 0, len(nodes))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, dep := range nodes[id].dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	return processed != len(nodes)
}

// DAG is a directed acyclic graph of tool calls. Construct via Analyze.
type DAG struct {
	nodes map[string]*dagNode
	order []string // insertion order, for stable iteration
}

// dagNode is an internal DAG node tracking dependencies and dependents.
type dagNode struct {
	call         *ToolCall
	dependents   []string // IDs that depend on this node
	dependencies int      // number of unresolved dependencies
}

// TopologicalLayers partitions the DAG into layers such that:
//   - layer 0 contains all calls with no dependencies
//   - layer N contains calls whose dependencies are all in layers 0..N-1
//
// Calls within the same layer have no inter-dependencies and can run in
// parallel. The number of layers equals the longest dependency chain.
// Returns an empty slice (no error) for an empty DAG.
func (d *DAG) TopologicalLayers() ([][]ToolCall, error) {
	if d == nil || len(d.nodes) == 0 {
		return nil, nil
	}

	// Copy in-degrees so repeated calls remain safe.
	inDegree := make(map[string]int, len(d.nodes))
	for id, n := range d.nodes {
		inDegree[id] = n.dependencies
	}

	var layers [][]ToolCall
	processed := 0
	for len(inDegree) > 0 {
		// Find all nodes with no remaining dependencies, preserving insertion
		// order for deterministic output.
		var layer []ToolCall
		for _, id := range d.order {
			if deg, ok := inDegree[id]; ok && deg == 0 {
				layer = append(layer, *d.nodes[id].call)
			}
		}
		if len(layer) == 0 {
			// Should not happen: cycles are rejected in Analyze. Defensive guard.
			return layers, ErrCycleDetected
		}

		// Remove processed nodes and decrement dependents' in-degree.
		for i := range layer {
			id := layer[i].ID
			delete(inDegree, id)
			for _, dep := range d.nodes[id].dependents {
				if _, ok := inDegree[dep]; ok {
					inDegree[dep]--
				}
			}
		}
		layers = append(layers, layer)
		processed += len(layer)
	}

	if processed != len(d.nodes) {
		return layers, ErrCycleDetected
	}
	return layers, nil
}
