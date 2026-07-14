package memory

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// mockTraverser is a test stub for biz.L4GraphTraverser that returns a
// pre-constructed MemoryGraphTraversal (or an error).
type mockTraverser struct {
	graph *biz.MemoryGraphTraversal
	err   error
}

func (m *mockTraverser) GraphTraverseCTE(ctx context.Context, centerID string, hops, topK int) (*biz.MemoryGraphTraversal, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.graph, nil
}

// buildLinearGraph builds A -0.8-> B -0.5-> C -0.9-> D.
func buildLinearGraph() *biz.MemoryGraphTraversal {
	return &biz.MemoryGraphTraversal{
		CenterID: "A",
		Hops:     3,
		Nodes: []biz.MemoryGraphNode{
			{ID: "A", Hop: 0, Activation: 1.0},
			{ID: "B", Hop: 1, Activation: 0.8},
			{ID: "C", Hop: 2, Activation: 0.4},
			{ID: "D", Hop: 3, Activation: 0.36},
		},
		Edges: []biz.MemoryGraphEdge{
			{SourceID: "A", TargetID: "B", RelationType: "CAUSAL", Weight: 0.8},
			{SourceID: "B", TargetID: "C", RelationType: "CAUSAL", Weight: 0.5},
			{SourceID: "C", TargetID: "D", RelationType: "CAUSAL", Weight: 0.9},
		},
	}
}

// TestSpreadingActivation_LinearChain verifies the basic propagation math.
// A (1.0) -0.8-> B -0.5-> C -0.9-> D
// hop1: B = 1.0 * 0.8 * decay(1) = 0.8
// hop2: C = 0.8 * 0.5 * decay(2) = 0.8 * 0.5 * 0.7 = 0.28
// hop3: D = 0.28 * 0.9 * decay(3) = 0.28 * 0.9 * 0.49 = 0.12348
func TestSpreadingActivation_LinearChain(t *testing.T) {
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: buildLinearGraph()}, nil)
	results, err := engine.SpreadingActivation(context.Background(), "A", 3, 10)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d: %+v", len(results), results)
	}

	// First result should be center (activation=1.0).
	if results[0].NodeID != "A" {
		t.Errorf("expected A first, got %q", results[0].NodeID)
	}
	if abs(results[0].Activation-1.0) > 1e-9 {
		t.Errorf("A activation: got %v, want 1.0", results[0].Activation)
	}
	if results[0].HopCount != 0 {
		t.Errorf("A hop: got %d, want 0", results[0].HopCount)
	}

	// Find B, C, D in results.
	actByID := make(map[string]float64)
	hopByID := make(map[string]int)
	for _, r := range results {
		actByID[r.NodeID] = r.Activation
		hopByID[r.NodeID] = r.HopCount
	}

	// B: 1.0 * 0.8 * decay(1)=1.0 = 0.8
	if abs(actByID["B"]-0.8) > 1e-9 {
		t.Errorf("B activation: got %v, want 0.8", actByID["B"])
	}
	if hopByID["B"] != 1 {
		t.Errorf("B hop: got %d, want 1", hopByID["B"])
	}

	// C: 0.8 * 0.5 * decay(2)=0.7 = 0.28
	if abs(actByID["C"]-0.28) > 1e-9 {
		t.Errorf("C activation: got %v, want 0.28", actByID["C"])
	}
	if hopByID["C"] != 2 {
		t.Errorf("C hop: got %d, want 2", hopByID["C"])
	}

	// D: 0.28 * 0.9 * decay(3)=0.49 = 0.12348
	if abs(actByID["D"]-0.12348) > 1e-9 {
		t.Errorf("D activation: got %v, want 0.12348", actByID["D"])
	}
	if hopByID["D"] != 3 {
		t.Errorf("D hop: got %d, want 3", hopByID["D"])
	}
}

// TestSpreadingActivation_TopKPruning verifies that topK limits per-hop nodes.
func TestSpreadingActivation_TopKPruning(t *testing.T) {
	// Center A has 5 neighbors with different weights.
	graph := &biz.MemoryGraphTraversal{
		CenterID: "A",
		Hops:     1,
		Nodes: []biz.MemoryGraphNode{
			{ID: "A", Hop: 0, Activation: 1.0},
			{ID: "B", Hop: 1, Activation: 0.9},
			{ID: "C", Hop: 1, Activation: 0.7},
			{ID: "D", Hop: 1, Activation: 0.5},
			{ID: "E", Hop: 1, Activation: 0.3},
			{ID: "F", Hop: 1, Activation: 0.1},
		},
		Edges: []biz.MemoryGraphEdge{
			{SourceID: "A", TargetID: "B", RelationType: "RELATED_TO", Weight: 0.9},
			{SourceID: "A", TargetID: "C", RelationType: "RELATED_TO", Weight: 0.7},
			{SourceID: "A", TargetID: "D", RelationType: "RELATED_TO", Weight: 0.5},
			{SourceID: "A", TargetID: "E", RelationType: "RELATED_TO", Weight: 0.3},
			{SourceID: "A", TargetID: "F", RelationType: "RELATED_TO", Weight: 0.1},
		},
	}
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: graph}, nil)
	// topK=3 → center + top 2 neighbors (3 total).
	results, err := engine.SpreadingActivation(context.Background(), "A", 1, 3)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("topK=3: expected ≤3 results, got %d", len(results))
	}

	// F (weight=0.1 → activation=0.1*1.0=0.1) should be pruned.
	for _, r := range results {
		if r.NodeID == "F" {
			t.Error("F (lowest activation) should be pruned by topK=3")
		}
	}
}

// TestSpreadingActivation_InhibitEdge verifies that INHIBIT edges do not
// propagate activation.
func TestSpreadingActivation_InhibitEdge(t *testing.T) {
	// A -INHIBIT-> B should not propagate activation to B.
	// A -CAUSAL-> C should propagate normally.
	graph := &biz.MemoryGraphTraversal{
		CenterID: "A",
		Hops:     1,
		Nodes: []biz.MemoryGraphNode{
			{ID: "A", Hop: 0, Activation: 1.0},
			{ID: "B", Hop: 1, Activation: 0},
			{ID: "C", Hop: 1, Activation: 0.8},
		},
		Edges: []biz.MemoryGraphEdge{
			{SourceID: "A", TargetID: "B", RelationType: "INHIBIT", Weight: 0.9},
			{SourceID: "A", TargetID: "C", RelationType: "CAUSAL", Weight: 0.8},
		},
	}
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: graph}, nil)
	results, err := engine.SpreadingActivation(context.Background(), "A", 1, 10)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}

	foundB := false
	foundC := false
	for _, r := range results {
		if r.NodeID == "B" {
			foundB = true
			if r.Activation > 0 {
				t.Errorf("B should have 0 activation (INHIBIT edge), got %v", r.Activation)
			}
		}
		if r.NodeID == "C" {
			foundC = true
			if r.Activation <= 0 {
				t.Errorf("C should have positive activation (CAUSAL edge), got %v", r.Activation)
			}
		}
	}
	if foundB {
		t.Error("B should not appear in results (INHIBIT blocks propagation)")
	}
	if !foundC {
		t.Error("C should appear in results (CAUSAL propagates)")
	}
}

// TestSpreadingActivation_ThresholdFilter verifies that activation < 0.01
// is not propagated.
func TestSpreadingActivation_ThresholdFilter(t *testing.T) {
	// A -0.005-> B: weight too small, propagated = 1.0 * 0.005 * 1.0 = 0.005 < 0.01
	graph := &biz.MemoryGraphTraversal{
		CenterID: "A",
		Hops:     1,
		Nodes: []biz.MemoryGraphNode{
			{ID: "A", Hop: 0, Activation: 1.0},
			{ID: "B", Hop: 1, Activation: 0.005},
		},
		Edges: []biz.MemoryGraphEdge{
			{SourceID: "A", TargetID: "B", RelationType: "RELATED_TO", Weight: 0.005},
		},
	}
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: graph}, nil)
	results, err := engine.SpreadingActivation(context.Background(), "A", 1, 10)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}

	// Only A should be in results (B's propagation < threshold).
	if len(results) != 1 {
		t.Errorf("expected 1 result (only A), got %d", len(results))
	}
	if len(results) > 0 && results[0].NodeID != "A" {
		t.Errorf("expected A, got %q", results[0].NodeID)
	}
}

// TestSpreadingActivation_DecayFactor verifies the decay factor calculation.
func TestSpreadingActivation_DecayFactor(t *testing.T) {
	cases := []struct {
		hop  int
		want float64
	}{
		{1, 1.0},
		{2, 0.7},
		{3, 0.49},
		{4, 0.343},
	}
	for _, c := range cases {
		got := decayFactor(c.hop)
		if abs(got-c.want) > 1e-9 {
			t.Errorf("decayFactor(%d): got %v, want %v", c.hop, got, c.want)
		}
	}
}

// TestSpreadingActivation_EmptyGraph verifies behavior with no nodes.
func TestSpreadingActivation_EmptyGraph(t *testing.T) {
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: &biz.MemoryGraphTraversal{CenterID: "A"}}, nil)
	results, err := engine.SpreadingActivation(context.Background(), "A", 3, 10)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty graph, got %d", len(results))
	}
}

// TestSpreadingActivation_TraverserError verifies error propagation.
func TestSpreadingActivation_TraverserError(t *testing.T) {
	expectedErr := errors.New("CTE failed")
	engine := NewSpreadingActivationEngine(&mockTraverser{err: expectedErr}, nil)
	_, err := engine.SpreadingActivation(context.Background(), "A", 3, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestSpreadingActivation_DefaultParameters verifies defaults for hops and topK.
func TestSpreadingActivation_DefaultParameters(t *testing.T) {
	graph := &biz.MemoryGraphTraversal{
		CenterID: "A",
		Hops:     3,
		Nodes:    []biz.MemoryGraphNode{{ID: "A", Hop: 0, Activation: 1.0}},
	}
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: graph}, nil)
	// hops=0 and topK=0 should use defaults (3 and 20).
	results, err := engine.SpreadingActivation(context.Background(), "A", 0, 0)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (center only), got %d", len(results))
	}
	if results[0].NodeID != "A" {
		t.Errorf("expected A, got %q", results[0].NodeID)
	}
}

// TestSpreadingActivation_NilEngine verifies nil safety.
func TestSpreadingActivation_NilEngine(t *testing.T) {
	var engine *SpreadingActivationEngine
	results, err := engine.SpreadingActivation(context.Background(), "A", 3, 10)
	if err != nil {
		t.Errorf("expected nil error for nil engine, got %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for nil engine, got %v", results)
	}
}

// TestSpreadingActivation_TopKFilter verifies the topKFilter helper directly.
func TestSpreadingActivation_TopKFilter(t *testing.T) {
	m := map[string]float64{
		"a": 0.9, "b": 0.5, "c": 0.3, "d": 0.1,
	}
	out := topKFilter(m, 2)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	if _, ok := out["a"]; !ok {
		t.Error("a should be in top 2")
	}
	if _, ok := out["b"]; !ok {
		t.Error("b should be in top 2")
	}
	if _, ok := out["d"]; ok {
		t.Error("d should be pruned")
	}
}

// TestSpreadingActivation_ActivationPath verifies that the path is recorded.
func TestSpreadingActivation_ActivationPath(t *testing.T) {
	engine := NewSpreadingActivationEngine(&mockTraverser{graph: buildLinearGraph()}, nil)
	results, err := engine.SpreadingActivation(context.Background(), "A", 3, 10)
	if err != nil {
		t.Fatalf("SpreadingActivation: %v", err)
	}

	for _, r := range results {
		if r.NodeID == "A" {
			// Center has no path.
			if len(r.ActivationPath) != 0 {
				t.Errorf("A should have empty path, got %d steps", len(r.ActivationPath))
			}
			continue
		}
		// Non-center nodes should have a path.
		if len(r.ActivationPath) == 0 {
			t.Errorf("node %q should have a path", r.NodeID)
		}
		// Verify path step structure.
		for _, step := range r.ActivationPath {
			if step.FromNodeID == "" || step.ToNodeID == "" {
				t.Errorf("path step has empty node IDs: %+v", step)
			}
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
