package data_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
)

// openTestDBForUnifiedGraph creates the tables needed for memory-center
// unified graph / layer overview queries: memory_entities + memory_relations
// (via openTestDBForNeuron) plus memory_episodes.
func openTestDBForUnifiedGraph(t *testing.T) (*data.Data, *ent.Client) {
	t.Helper()
	d, client := openTestDBForNeuron(t)
	ctx := context.Background()
	_, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS memory_episodes (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL DEFAULT '',
 team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '', l1_task_id TEXT NOT NULL DEFAULT '',
 episode_kind TEXT NOT NULL DEFAULT 'task', title TEXT NOT NULL, goal TEXT NOT NULL DEFAULT '',
 outcome TEXT NOT NULL DEFAULT '', outcome_summary TEXT NOT NULL DEFAULT '', result_preview TEXT NOT NULL DEFAULT '',
 failure_reason TEXT NOT NULL DEFAULT '', importance REAL NOT NULL DEFAULT 0.5, confidence REAL NOT NULL DEFAULT 0.7,
 user_feedback TEXT NOT NULL DEFAULT '', critic_score REAL NOT NULL DEFAULT -1,
 span_count INTEGER NOT NULL DEFAULT 0, message_count INTEGER NOT NULL DEFAULT 0,
 tool_call_count INTEGER NOT NULL DEFAULT 0, skill_call_count INTEGER NOT NULL DEFAULT 0,
 mcp_call_count INTEGER NOT NULL DEFAULT 0, total_tokens INTEGER NOT NULL DEFAULT 0,
 total_cost_micro_usd INTEGER NOT NULL DEFAULT 0, duration_ms INTEGER NOT NULL DEFAULT 0,
 l1_snapshot_json TEXT NOT NULL DEFAULT '{}', key_decisions_json TEXT NOT NULL DEFAULT '[]',
 key_artifacts_json TEXT NOT NULL DEFAULT '[]',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '',
 embedding_dim INTEGER NOT NULL DEFAULT 0, embedding_blob BYTEA, embedding_norm REAL NOT NULL DEFAULT 0,
 consolidation_status TEXT NOT NULL DEFAULT 'consolidated', consolidated_at TEXT NOT NULL DEFAULT '',
 consolidated_l3_count INTEGER NOT NULL DEFAULT 0, consolidated_l4_count INTEGER NOT NULL DEFAULT 0,
 metadata_json TEXT NOT NULL DEFAULT '{}', started_at TEXT NOT NULL DEFAULT '', ended_at TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '')`)
	if err != nil {
		t.Fatalf("create memory_episodes: %v", err)
	}
	return d, client
}

func insertUnifiedEpisode(t *testing.T, client *ent.Client, id, agentID, sessionID, title, createdAt string) {
	t.Helper()
	_, err := client.ExecContext(context.Background(), pgRebind(`INSERT INTO memory_episodes (
 id, session_id, agent_id, episode_kind, title, outcome_summary, importance,
 consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		id, sessionID, agentID, "task", title, "summary of "+title, 0.6,
		"consolidated", 0, "{}", createdAt, createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert episode %s: %v", id, err)
	}
}

func insertUnifiedEntity(t *testing.T, client *ent.Client, id, scopeType, scopeID, name string) {
	t.Helper()
	_, err := client.ExecContext(context.Background(), pgRebind(`INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?)`),
		id, scopeType, scopeID, "concept", name, name,
		"active", "2026-07-25T00:00:00Z", "2026-07-25T00:00:00Z")
	if err != nil {
		t.Fatalf("insert entity %s: %v", id, err)
	}
}

func insertUnifiedRelation(t *testing.T, client *ent.Client, id, scopeID, source, target, relType, status string, weight float64) {
	t.Helper()
	_, err := client.ExecContext(context.Background(), pgRebind(`INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, importance,
 status, created_at, updated_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
		id, "agent", scopeID, source, target, relType,
		weight, 0.8, 0.5, status, "2026-07-25T00:00:00Z", "2026-07-25T00:00:00Z")
	if err != nil {
		t.Fatalf("insert relation %s: %v", id, err)
	}
}

// TestMemoryUnifiedGraph_ListEpisodeRowsAdmin verifies paginated episode
// browsing with total + today-count aggregation.
func TestMemoryUnifiedGraph_ListEpisodeRowsAdmin(t *testing.T) {
	d, client := openTestDBForUnifiedGraph(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)

	insertUnifiedEpisode(t, client, "ep-1", "agent-1", "sess-1", "Alpha", now)
	insertUnifiedEpisode(t, client, "ep-2", "agent-1", "sess-1", "Bravo", now)
	insertUnifiedEpisode(t, client, "ep-3", "agent-1", "sess-2", "Charlie", old)
	insertUnifiedEpisode(t, client, "ep-4", "agent-2", "sess-9", "Delta", now)

	reader := data.NewL2EpisodeAdminReader(d, nil)

	// All sessions for agent-1.
	rows, total, today, err := reader.ListEpisodeRowsAdmin(ctx, "agent-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListEpisodeRowsAdmin: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if today != 2 {
		t.Errorf("today: got %d, want 2", today)
	}
	if len(rows) != 3 {
		t.Fatalf("rows: got %d, want 3", len(rows))
	}
	var first map[string]any
	if err := json.Unmarshal(rows[0], &first); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	if first["id"] == nil || first["title"] == nil {
		t.Errorf("row missing id/title: %v", first)
	}

	// Session filter.
	_, total, today, err = reader.ListEpisodeRowsAdmin(ctx, "agent-1", "sess-1", 10, 0)
	if err != nil {
		t.Fatalf("ListEpisodeRowsAdmin(sess-1): %v", err)
	}
	if total != 2 || today != 2 {
		t.Errorf("sess-1: got total=%d today=%d, want 2/2", total, today)
	}

	// Pagination.
	rows, total, _, err = reader.ListEpisodeRowsAdmin(ctx, "agent-1", "", 1, 1)
	if err != nil {
		t.Fatalf("ListEpisodeRowsAdmin(paged): %v", err)
	}
	if total != 3 {
		t.Errorf("paged total: got %d, want 3", total)
	}
	if len(rows) != 1 {
		t.Errorf("paged rows: got %d, want 1", len(rows))
	}
}

// TestMemoryUnifiedGraph_ListEpisodeRowsByIDs verifies batch fetch by IDs.
func TestMemoryUnifiedGraph_ListEpisodeRowsByIDs(t *testing.T) {
	d, client := openTestDBForUnifiedGraph(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertUnifiedEpisode(t, client, "ep-1", "agent-1", "sess-1", "Alpha", now)
	insertUnifiedEpisode(t, client, "ep-2", "agent-1", "sess-1", "Bravo", now)
	insertUnifiedEpisode(t, client, "ep-3", "agent-1", "sess-1", "Charlie", now)

	reader := data.NewL2EpisodeAdminReader(d, nil)

	rows, err := reader.ListEpisodeRowsByIDs(ctx, []string{"ep-1", "ep-3", "missing"})
	if err != nil {
		t.Fatalf("ListEpisodeRowsByIDs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	seen := map[string]bool{}
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		seen[m["id"].(string)] = true
	}
	if !seen["ep-1"] || !seen["ep-3"] {
		t.Errorf("expected ep-1 and ep-3, got %v", seen)
	}

	// Empty input → no rows, no error.
	rows, err = reader.ListEpisodeRowsByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ListEpisodeRowsByIDs(nil): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("nil ids: got %d rows, want 0", len(rows))
	}
}

// TestMemoryUnifiedGraph_RelationAdminReader verifies active-relation count,
// full row listing, and top-connected entity selection.
func TestMemoryUnifiedGraph_RelationAdminReader(t *testing.T) {
	d, client := openTestDBForUnifiedGraph(t)
	ctx := context.Background()

	for _, id := range []string{"ent-A", "ent-B", "ent-C"} {
		insertUnifiedEntity(t, client, id, "agent", "agent-1", id)
	}
	insertUnifiedEntity(t, client, "ent-X", "agent", "agent-2", "ent-X")
	insertUnifiedEntity(t, client, "ent-Y", "agent", "agent-2", "ent-Y")

	insertUnifiedRelation(t, client, "rel-1", "agent-1", "ent-A", "ent-B", "RELATED_TO", "active", 0.9)
	insertUnifiedRelation(t, client, "rel-2", "agent-1", "ent-B", "ent-C", "CAUSAL", "active", 0.4)
	insertUnifiedRelation(t, client, "rel-3", "agent-1", "ent-A", "ent-C", "INHIBIT", "archived", 0.8)
	insertUnifiedRelation(t, client, "rel-4", "agent-2", "ent-X", "ent-Y", "RELATED_TO", "active", 0.7)

	reader := data.NewL4RelationAdminReader(d)

	// CountActiveRelations: scope agent-1 has 2 active (archived excluded).
	count, err := reader.CountActiveRelations(ctx, "agent", "agent-1")
	if err != nil {
		t.Fatalf("CountActiveRelations: %v", err)
	}
	if count != 2 {
		t.Errorf("count: got %d, want 2", count)
	}

	// ListActiveRelationRows: 2 rows for agent-1 scope.
	rows, err := reader.ListActiveRelationRows(ctx, "agent", "agent-1")
	if err != nil {
		t.Fatalf("ListActiveRelationRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m["source_id"] == nil || m["target_id"] == nil || m["relation_type"] == nil {
			t.Errorf("relation row missing fields: %v", m)
		}
		if m["status"] != "active" {
			t.Errorf("status: got %v, want active", m["status"])
		}
	}

	// TopConnectedEntityID: ent-B (rel-1 target + rel-2 source = 2 hits).
	top, err := reader.TopConnectedEntityID(ctx, "agent", "agent-1")
	if err != nil {
		t.Fatalf("TopConnectedEntityID: %v", err)
	}
	if top != "ent-B" {
		t.Errorf("top: got %q, want ent-B", top)
	}

	// Empty scope → empty string, no error.
	top, err = reader.TopConnectedEntityID(ctx, "agent", "agent-nope")
	if err != nil {
		t.Fatalf("TopConnectedEntityID(empty): %v", err)
	}
	if top != "" {
		t.Errorf("empty scope: got %q, want \"\"", top)
	}
}
