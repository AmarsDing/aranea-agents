package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
)

// asTraverser casts the SessionAdminStore to biz.L4GraphTraverser. The
// underlying *l4EntityRepo implements both interfaces.
func asTraverser(t *testing.T, store biz.SessionAdminStore) biz.L4GraphTraverser {
	t.Helper()
	tr, ok := store.(biz.L4GraphTraverser)
	if !ok {
		t.Fatalf("store does not implement biz.L4GraphTraverser: %T", store)
	}
	return tr
}

// TestGraphTraverseCTE_LinearChain verifies that GraphTraverseCTE correctly
// traverses a linear chain A -> B -> C -> D with weighted edges and propagates
// activation = parent_activation * edge_weight.
//
// Graph:
//
//	A --0.8--> B --0.5--> C --0.9--> D
//
// Expected activations (from A, hops=3):
//   - A: 1.0 (center, hop=0)
//   - B: 1.0 * 0.8 = 0.8 (hop=1)
//   - C: 0.8 * 0.5 = 0.4 (hop=2)
//   - D: 0.4 * 0.9 = 0.36 (hop=3)
func TestGraphTraverseCTE_LinearChain(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	entities := []struct {
		id, entityType, name string
	}{
		{"ent-A", "concept", "Alpha"},
		{"ent-B", "concept", "Bravo"},
		{"ent-C", "concept", "Charlie"},
		{"ent-D", "concept", "Delta"},
	}
	for _, e := range entities {
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
			e.id, "agent", "agent-1", e.entityType, e.name, e.name,
			"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert entity %s: %v", e.id, err)
		}
	}

	relations := []struct {
		id, source, target string
		weight             float64
	}{
		{"rel-AB", "ent-A", "ent-B", 0.8},
		{"rel-BC", "ent-B", "ent-C", 0.5},
		{"rel-CD", "ent-C", "ent-D", 0.9},
	}
	for _, r := range relations {
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, importance,
 status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			r.id, "agent", "agent-1", r.source, r.target, "CAUSAL",
			r.weight, 0.8, 0.5, "active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert relation %s: %v", r.id, err)
		}
	}

	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))
	result, err := trav.GraphTraverseCTE(ctx, "ent-A", 3, 10)
	if err != nil {
		t.Fatalf("GraphTraverseCTE: %v", err)
	}

	if result.CenterID != "ent-A" {
		t.Errorf("CenterID: got %q, want ent-A", result.CenterID)
	}
	if result.Hops != 3 {
		t.Errorf("Hops: got %d, want 3", result.Hops)
	}
	if len(result.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}

	type nodeInfo struct {
		activation float64
		hop        int
		entityType string
		name       string
	}
	got := make(map[string]nodeInfo)
	for _, n := range result.Nodes {
		got[n.ID] = nodeInfo{n.Activation, n.Hop, n.EntityType, n.Name}
	}

	expected := map[string]nodeInfo{
		"ent-A": {1.0, 0, "concept", "Alpha"},
		"ent-B": {0.8, 1, "concept", "Bravo"},
		"ent-C": {0.4, 2, "concept", "Charlie"},
		"ent-D": {0.36, 3, "concept", "Delta"},
	}
	for id, want := range expected {
		g, ok := got[id]
		if !ok {
			t.Errorf("missing node %q in result", id)
			continue
		}
		// Tolerance 1e-6: weight columns are REAL (float4 on Postgres, ~7
		// significant digits), so exact float8 equality is not achievable.
		if abs(g.activation-want.activation) > 1e-6 {
			t.Errorf("node %q activation: got %v, want %v", id, g.activation, want.activation)
		}
		if g.hop != want.hop {
			t.Errorf("node %q hop: got %d, want %d", id, g.hop, want.hop)
		}
		if g.entityType != want.entityType {
			t.Errorf("node %q entityType: got %q, want %q", id, g.entityType, want.entityType)
		}
		if g.name != want.name {
			t.Errorf("node %q name: got %q, want %q", id, g.name, want.name)
		}
	}

	if len(result.Edges) != 3 {
		t.Errorf("expected 3 edges, got %d: %+v", len(result.Edges), result.Edges)
	}

	neighbors := result.Neighbors("ent-B")
	if len(neighbors) != 2 {
		t.Errorf("B should have 2 neighbors, got %d", len(neighbors))
	}

	if _, ok := result.NodeByID("ent-C"); !ok {
		t.Error("NodeByID(ent-C) returned false")
	}
	if _, ok := result.NodeByID("nonexistent"); ok {
		t.Error("NodeByID(nonexistent) should return false")
	}
}

// TestGraphTraverseCTE_TopKLimit verifies that topK limits the number of
// returned nodes to the highest-activation ones.
func TestGraphTraverseCTE_TopKLimit(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
		"center", "agent", "agent-1", "concept", "Center", "center",
		"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
		t.Fatalf("insert center: %v", err)
	}

	neighbors := []struct {
		id, name string
		weight   float64
	}{
		{"n1", "Neighbor1", 0.9},
		{"n2", "Neighbor2", 0.7},
		{"n3", "Neighbor3", 0.5},
		{"n4", "Neighbor4", 0.3},
		{"n5", "Neighbor5", 0.1},
	}
	for _, n := range neighbors {
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
			n.id, "agent", "agent-1", "concept", n.name, n.name,
			"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert entity %s: %v", n.id, err)
		}
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, importance,
 status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			"rel-"+n.id, "agent", "agent-1", "center", n.id, "RELATED_TO",
			n.weight, 0.8, 0.5, "active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert relation for %s: %v", n.id, err)
		}
	}

	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))
	result, err := trav.GraphTraverseCTE(ctx, "center", 1, 3)
	if err != nil {
		t.Fatalf("GraphTraverseCTE: %v", err)
	}

	if len(result.Nodes) > 3 {
		t.Errorf("topK=3: expected ≤3 nodes, got %d", len(result.Nodes))
	}
	if len(result.Nodes) > 0 && result.Nodes[0].ID != "center" {
		t.Errorf("expected center first, got %q", result.Nodes[0].ID)
	}

	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.ID] = true
	}
	if nodeIDs["n5"] {
		t.Error("n5 (weight=0.1) should be pruned by topK=3")
	}
}

// TestGraphTraverseCTE_HopLimit verifies that hops limits traversal depth.
func TestGraphTraverseCTE_HopLimit(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	for _, id := range []string{"ent-A", "ent-B", "ent-C"} {
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
			id, "agent", "agent-1", "concept", id, id,
			"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert entity %s: %v", id, err)
		}
	}
	for _, r := range []struct {
		id, source, target string
	}{
		{"rel-AB", "ent-A", "ent-B"},
		{"rel-BC", "ent-B", "ent-C"},
	} {
		if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, importance,
 status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			r.id, "agent", "agent-1", r.source, r.target, "CAUSAL",
			0.8, 0.8, 0.5, "active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert relation %s: %v", r.id, err)
		}
	}

	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))

	result, err := trav.GraphTraverseCTE(ctx, "ent-A", 1, 10)
	if err != nil {
		t.Fatalf("GraphTraverseCTE hops=1: %v", err)
	}
	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.ID] = true
	}
	if !nodeIDs["ent-A"] || !nodeIDs["ent-B"] {
		t.Errorf("hops=1: expected A and B, got %v", nodeIDs)
	}
	if nodeIDs["ent-C"] {
		t.Error("hops=1: C should not be reachable (2 hops away)")
	}
}

// TestGraphTraverseCTE_EmptyGraph verifies behavior when center has no relations.
func TestGraphTraverseCTE_EmptyGraph(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
		"lonely", "agent", "agent-1", "concept", "Lonely", "lonely",
		"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))
	result, err := trav.GraphTraverseCTE(ctx, "lonely", 3, 10)
	if err != nil {
		t.Fatalf("GraphTraverseCTE: %v", err)
	}

	if len(result.Nodes) != 1 {
		t.Errorf("expected 1 node (center only), got %d", len(result.Nodes))
	}
	if len(result.Nodes) > 0 && result.Nodes[0].ID != "lonely" {
		t.Errorf("expected center node, got %q", result.Nodes[0].ID)
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

// TestGraphTraverseCTE_NilCenterID verifies input validation.
func TestGraphTraverseCTE_NilCenterID(t *testing.T) {
	d, _ := openTestDBForNeuron(t)
	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))

	_, err := trav.GraphTraverseCTE(context.Background(), "", 3, 10)
	if err == nil {
		t.Error("expected error for empty centerID, got nil")
	}
}

// TestGraphTraverseCTE_DefaultParameters verifies that hops<=0 defaults to 3
// and topK<=0 defaults to 20.
func TestGraphTraverseCTE_DefaultParameters(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	if _, err := client.ExecContext(ctx, pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
		"solo", "agent", "agent-1", "concept", "Solo", "solo",
		"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	trav := asTraverser(t, data.NewSessionAdminStoreAdapter(d, nil))

	result, err := trav.GraphTraverseCTE(ctx, "solo", 0, 0)
	if err != nil {
		t.Fatalf("GraphTraverseCTE: %v", err)
	}
	if result.Hops != 3 {
		t.Errorf("default hops: got %d, want 3", result.Hops)
	}
}
