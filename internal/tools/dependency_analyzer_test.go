package tools

import (
	"errors"
	"testing"
)

// TestDependencyAnalyzer_NoDependencies verifies that calls without any
// DependsOn relations are placed into a single layer (fully parallel).
func TestDependencyAnalyzer_NoDependencies(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "read_file"},
		{ID: "b", Name: "read_file"},
		{ID: "c", Name: "read_file"},
	}

	a := NewDependencyAnalyzer()
	dag, err := a.Analyze(calls)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	layers, err := dag.TopologicalLayers()
	if err != nil {
		t.Fatalf("TopologicalLayers returned error: %v", err)
	}

	if len(layers) != 1 {
		t.Fatalf("expected 1 layer for independent calls, got %d", len(layers))
	}
	if len(layers[0]) != 3 {
		t.Fatalf("expected 3 calls in layer 0, got %d", len(layers[0]))
	}
}

// TestDependencyAnalyzer_ChainedDependencies verifies that a -> b -> c
// dependency chain produces 3 sequential layers.
func TestDependencyAnalyzer_ChainedDependencies(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "step1"},
		{ID: "b", Name: "step2", DependsOn: []string{"a"}},
		{ID: "c", Name: "step3", DependsOn: []string{"b"}},
	}

	a := NewDependencyAnalyzer()
	dag, err := a.Analyze(calls)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	layers, err := dag.TopologicalLayers()
	if err != nil {
		t.Fatalf("TopologicalLayers returned error: %v", err)
	}

	if len(layers) != 3 {
		t.Fatalf("expected 3 layers for chain a->b->c, got %d", len(layers))
	}
	if layers[0][0].ID != "a" {
		t.Errorf("layer 0 should contain 'a', got %s", layers[0][0].ID)
	}
	if layers[1][0].ID != "b" {
		t.Errorf("layer 1 should contain 'b', got %s", layers[1][0].ID)
	}
	if layers[2][0].ID != "c" {
		t.Errorf("layer 2 should contain 'c', got %s", layers[2][0].ID)
	}
}

// TestDependencyAnalyzer_DiamondDependency verifies a diamond dependency:
//
//	a -> b, a -> c, b -> d, c -> d
//
// produces 3 layers: [a], [b,c], [d].
func TestDependencyAnalyzer_DiamondDependency(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "root"},
		{ID: "b", Name: "left", DependsOn: []string{"a"}},
		{ID: "c", Name: "right", DependsOn: []string{"a"}},
		{ID: "d", Name: "join", DependsOn: []string{"b", "c"}},
	}

	a := NewDependencyAnalyzer()
	dag, err := a.Analyze(calls)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	layers, err := dag.TopologicalLayers()
	if err != nil {
		t.Fatalf("TopologicalLayers returned error: %v", err)
	}

	if len(layers) != 3 {
		t.Fatalf("expected 3 layers for diamond, got %d", len(layers))
	}
	if len(layers[0]) != 1 || layers[0][0].ID != "a" {
		t.Errorf("layer 0 should be [a], got %v", layerIDs(layers[0]))
	}
	if len(layers[1]) != 2 {
		t.Errorf("layer 1 should have 2 calls, got %d", len(layers[1]))
	}
	if len(layers[2]) != 1 || layers[2][0].ID != "d" {
		t.Errorf("layer 2 should be [d], got %v", layerIDs(layers[2]))
	}
}

// TestDependencyAnalyzer_CycleDetected verifies that a dependency cycle
// returns an error from Analyze.
func TestDependencyAnalyzer_CycleDetected(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"c"}},
		{ID: "b", Name: "y", DependsOn: []string{"a"}},
		{ID: "c", Name: "z", DependsOn: []string{"b"}},
	}

	a := NewDependencyAnalyzer()
	_, err := a.Analyze(calls)
	if err == nil {
		t.Fatal("expected error for cycle a->c->b->a, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestDependencyAnalyzer_SelfCycle verifies a self-referencing call is rejected.
func TestDependencyAnalyzer_SelfCycle(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"a"}},
	}

	a := NewDependencyAnalyzer()
	_, err := a.Analyze(calls)
	if err == nil {
		t.Fatal("expected error for self-cycle, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestDependencyAnalyzer_MissingDependency verifies that referencing a
// non-existent call ID returns an error.
func TestDependencyAnalyzer_MissingDependency(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"nonexistent"}},
	}

	a := NewDependencyAnalyzer()
	_, err := a.Analyze(calls)
	if err == nil {
		t.Fatal("expected error for missing dependency, got nil")
	}
	if !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("expected ErrMissingDependency, got %v", err)
	}
}

// TestDependencyAnalyzer_DuplicateID verifies that duplicate call IDs are rejected.
func TestDependencyAnalyzer_DuplicateID(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "a", Name: "y"},
	}

	a := NewDependencyAnalyzer()
	_, err := a.Analyze(calls)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !errors.Is(err, ErrDuplicateCallID) {
		t.Fatalf("expected ErrDuplicateCallID, got %v", err)
	}
}

// TestDependencyAnalyzer_EmptyInput verifies that empty input returns empty layers.
func TestDependencyAnalyzer_EmptyInput(t *testing.T) {
	a := NewDependencyAnalyzer()
	dag, err := a.Analyze(nil)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	layers, err := dag.TopologicalLayers()
	if err != nil {
		t.Fatalf("TopologicalLayers returned error: %v", err)
	}
	if len(layers) != 0 {
		t.Fatalf("expected 0 layers for empty input, got %d", len(layers))
	}
}

// TestDependencyAnalyzer_NilContext ensures Analyze does not require a context.
func TestDependencyAnalyzer_PreservesCallData(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "read_file", Arguments: []byte(`{"path":"/tmp"}`), IsolationStrategy: "worktree"},
		{ID: "b", Name: "write_file", DependsOn: []string{"a"}, IsolationStrategy: "transaction"},
	}

	a := NewDependencyAnalyzer()
	dag, err := a.Analyze(calls)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	layers, _ := dag.TopologicalLayers()
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	got := layers[0][0]
	if got.Name != "read_file" {
		t.Errorf("expected Name=read_file, got %s", got.Name)
	}
	if got.IsolationStrategy != "worktree" {
		t.Errorf("expected IsolationStrategy=worktree, got %s", got.IsolationStrategy)
	}
	if string(got.Arguments) != `{"path":"/tmp"}` {
		t.Errorf("expected Arguments preserved, got %s", string(got.Arguments))
	}
}

// layerIDs extracts IDs from a layer for assertion helpers.
func layerIDs(layer []ToolCall) []string {
	ids := make([]string, len(layer))
	for i, c := range layer {
		ids[i] = c.ID
	}
	return ids
}
