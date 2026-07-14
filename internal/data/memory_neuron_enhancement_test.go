package data_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// openTestDBForNeuron creates an in-memory SQLite DB with memory_entities and
// memory_relations tables matching the post-20261005 schema (neuron enhancement).
func openTestDBForNeuron(t *testing.T) (*data.Data, *ent.Client) {
	t.Helper()
	d, client := openTestDataForMemory(t)
	ctx := context.Background()
	// memory_entities is already created by openTestDataForMemory (with neuron columns).
	// Create memory_relations with neuron enhancement columns.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS memory_relations (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', workspace_id TEXT NOT NULL DEFAULT '',
 source_id TEXT NOT NULL, target_id TEXT NOT NULL, relation_type TEXT NOT NULL, bidirectional INTEGER NOT NULL DEFAULT 0,
 weight REAL NOT NULL DEFAULT 1.0, confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5, use_count INTEGER NOT NULL DEFAULT 0,
 attributes_json TEXT NOT NULL DEFAULT '{}', evidence_json TEXT NOT NULL DEFAULT '[]', status TEXT NOT NULL DEFAULT 'active', source_kind TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', valid_from TEXT NOT NULL DEFAULT '', valid_to TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 co_activation_count INTEGER NOT NULL DEFAULT 0, last_reinforced_at TEXT NOT NULL DEFAULT '', context_note TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, source_id, target_id, relation_type))`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return d, client
}

// TestNeuronEntityFields_VerifyScanOutput verifies that scanEntityRowJSON returns
// the 20261005 neuron enhancement fields (activation, activation_updated_at,
// source_type, valence, arousal) in the JSON output.
func TestNeuronEntityFields_VerifyScanOutput(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	// Insert an entity with non-default neuron values.
	_, err := client.ExecContext(ctx, `INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at,
 activation, activation_updated_at, source_type, valence, arousal
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"ent-neuron-1", "agent", "agent-1", "concept", "Neuron Test", "neuron test",
		"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z",
		0.85, "2026-07-14T12:00:00Z", "perception", 0.3, 0.7)
	if err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	// Query using the production scan path (l4EntityRepo.ListEntityRows via SessionAdminStore).
	store := data.NewSessionAdminStoreAdapter(d, nil)
	rows, _, err := store.ListEntityRows(ctx, "agent", "agent-1", "", "", "", "active", "", 10, 0)
	if err != nil {
		t.Fatalf("ListEntityRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify neuron enhancement fields are present with correct values.
	checks := []struct {
		key  string
		want any
	}{
		{"activation", 0.85},
		{"activation_updated_at", "2026-07-14T12:00:00Z"},
		{"source_type", "perception"},
		{"valence", 0.3},
		{"arousal", 0.7},
	}
	for _, c := range checks {
		got, ok := m[c.key]
		if !ok {
			t.Errorf("missing key %q in entity JSON output", c.key)
			continue
		}
		// JSON numbers unmarshal as float64.
		if f, isFloat := c.want.(float64); isFloat {
			gf, ok := got.(float64)
			if !ok {
				t.Errorf("key %q: expected float64, got %T (%v)", c.key, got, got)
				continue
			}
			if abs(gf-f) > 1e-9 {
				t.Errorf("key %q: got %v, want %v", c.key, got, c.want)
			}
		} else if got != c.want {
			t.Errorf("key %q: got %v, want %v", c.key, got, c.want)
		}
	}
}

// TestNeuronRelationFields_VerifyScanOutput verifies that scanRelationRowJSON
// returns the 20261005 neuron enhancement fields (co_activation_count,
// last_reinforced_at, context_note) in the JSON output.
func TestNeuronRelationFields_VerifyScanOutput(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	// Insert two entities to reference.
	for _, id := range []string{"ent-rel-a", "ent-rel-b"} {
		if _, err := client.ExecContext(ctx, `INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`,
			id, "agent", "agent-1", "concept", id, id, "active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z"); err != nil {
			t.Fatalf("insert entity %s: %v", id, err)
		}
	}

	// Insert a relation with non-default neuron values.
	_, err := client.ExecContext(ctx, `INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, importance,
 status, created_at, updated_at, co_activation_count, last_reinforced_at, context_note
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"rel-neuron-1", "agent", "agent-1", "ent-rel-a", "ent-rel-b", "CAUSAL",
		0.9, 0.8, 0.6, "active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z",
		3, "2026-07-14T12:00:00Z", "co-activated during quant task")
	if err != nil {
		t.Fatalf("insert relation: %v", err)
	}

	// Query using NeighborhoodJSON which uses scanRelationRowJSON.
	store := data.NewSessionAdminStoreAdapter(d, nil)
	body, err := store.NeighborhoodJSON(ctx, "ent-rel-a", 1, 10, "")
	if err != nil {
		t.Fatalf("NeighborhoodJSON: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal neighborhood: %v", err)
	}

	relations, ok := result["relations"].([]any)
	if !ok {
		t.Fatalf("expected relations array, got %T", result["relations"])
	}
	if len(relations) == 0 {
		t.Fatal("expected at least 1 relation, got 0")
	}

	// Relations are stored as [][]byte in the map, which json.Marshal encodes as
	// base64 strings. Decode the first relation to verify its fields.
	relB64, ok := relations[0].(string)
	if !ok {
		t.Fatalf("expected relation[0] to be base64 string, got %T", relations[0])
	}
	relBytes, err := base64.StdEncoding.DecodeString(relB64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	var rel map[string]any
	if err := json.Unmarshal(relBytes, &rel); err != nil {
		t.Fatalf("unmarshal relation: %v", err)
	}

	checks := []struct {
		key  string
		want any
	}{
		{"co_activation_count", float64(3)}, // JSON numbers unmarshal as float64
		{"last_reinforced_at", "2026-07-14T12:00:00Z"},
		{"context_note", "co-activated during quant task"},
	}
	for _, c := range checks {
		got, ok := rel[c.key]
		if !ok {
			t.Errorf("missing key %q in relation JSON output", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("key %q: got %v, want %v", c.key, got, c.want)
		}
	}
}

// TestNeuronFields_DefaultValues verifies that entities/relations inserted
// without explicit neuron field values get the correct defaults.
func TestNeuronFields_DefaultValues(t *testing.T) {
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()

	// Insert an entity with only required fields (no neuron columns specified).
	_, err := client.ExecContext(ctx, `INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`,
		"ent-default", "agent", "agent-1", "concept", "Default", "default",
		"active", "2026-07-14T00:00:00Z", "2026-07-14T00:00:00Z")
	if err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	store := data.NewSessionAdminStoreAdapter(d, nil)
	rows, _, err := store.ListEntityRows(ctx, "agent", "agent-1", "", "", "", "active", "", 10, 0)
	if err != nil {
		t.Fatalf("ListEntityRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Defaults: activation=0, activation_updated_at="", source_type="",
	// valence=0, arousal=0
	defaultChecks := []struct {
		key  string
		want any
	}{
		{"activation", float64(0)},
		{"activation_updated_at", ""},
		{"source_type", ""},
		{"valence", float64(0)},
		{"arousal", float64(0)},
	}
	for _, c := range defaultChecks {
		got, ok := m[c.key]
		if !ok {
			t.Errorf("missing key %q", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("key %q: got %v, want default %v", c.key, got, c.want)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Ensure loggateway noop is used (avoids deprecated Global()).
var _ = loggateway.NewNoop

