package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openNeighborhoodTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS memory_entities (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '',
 entity_type TEXT NOT NULL, name TEXT NOT NULL, name_normalized TEXT NOT NULL, aliases_json TEXT NOT NULL DEFAULT '[]',
 description TEXT NOT NULL DEFAULT '', attributes_json TEXT NOT NULL DEFAULT '{}',
 importance REAL NOT NULL DEFAULT 0.5, confidence REAL NOT NULL DEFAULT 0.7, use_count INTEGER NOT NULL DEFAULT 0, source_kind TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BLOB, embedding_norm REAL NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'active', merged_into TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, entity_type, name_normalized))`,
		`CREATE TABLE IF NOT EXISTS memory_relations (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', workspace_id TEXT NOT NULL DEFAULT '',
 source_id TEXT NOT NULL, target_id TEXT NOT NULL, relation_type TEXT NOT NULL,
 bidirectional INTEGER NOT NULL DEFAULT 0, weight REAL NOT NULL DEFAULT 1, confidence REAL NOT NULL DEFAULT 0.7,
 importance REAL NOT NULL DEFAULT 0.5, use_count INTEGER NOT NULL DEFAULT 0,
 attributes_json TEXT NOT NULL DEFAULT '{}', evidence_json TEXT NOT NULL DEFAULT '[]',
 status TEXT NOT NULL DEFAULT 'active', source_kind TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}',
 valid_from TEXT NOT NULL DEFAULT '', valid_to TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	st := NewStore(client)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	expired := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	for _, q := range []struct {
		id, name, etype string
	}{
		{"center", "Center", "person"},
		{"n1", "Neighbor", "person"},
	} {
		if _, err := client.ExecContext(ctx, `INSERT INTO memory_entities (
 id, scope_type, scope_id, entity_type, name, name_normalized, created_at, updated_at
) VALUES ('`+q.id+`','agent','ag1','`+q.etype+`','`+q.name+`', '`+q.name+`','`+now+`','`+now+`')`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.ExecContext(ctx, `INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, valid_from, valid_to, created_at, updated_at
) VALUES ('r-active','agent','ag1','center','n1','knows_as','`+expired+`','','`+now+`','`+now+`')`); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExecContext(ctx, `INSERT INTO memory_relations (
 id, scope_type, scope_id, source_id, target_id, relation_type, valid_from, valid_to, created_at, updated_at
) VALUES ('r-expired','agent','ag1','center','n1','knows_as','`+expired+`','`+expired+`','`+now+`','`+now+`')`); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestNeighborhoodJSON_QueryAtFiltersExpiredRelations(t *testing.T) {
	st := openNeighborhoodTestStore(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	raw, err := st.NeighborhoodJSON(ctx, "center", 1, 10, past)
	if err != nil {
		t.Fatal(err)
	}
	var nb struct {
		Relations []map[string]any `json:"relations"`
		Entities  []map[string]any `json:"entities"`
	}
	if err := json.Unmarshal(raw, &nb); err != nil {
		t.Fatal(err)
	}
	if len(nb.Relations) != 0 {
		t.Fatalf("expected no relations at past query_at, got %d", len(nb.Relations))
	}
}

func TestNeighborhoodJSON_EntityHopField(t *testing.T) {
	st := openNeighborhoodTestStore(t)
	ctx := context.Background()
	raw, err := st.NeighborhoodJSON(ctx, "center", 1, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	var nb struct {
		Entities []map[string]any `json:"entities"`
	}
	if err := json.Unmarshal(raw, &nb); err != nil {
		t.Fatal(err)
	}
	if len(nb.Entities) == 0 {
		t.Fatal("expected neighbor entity")
	}
	if hop, _ := nb.Entities[0]["hop"].(float64); int(hop) != 1 {
		t.Fatalf("expected hop=1, got %v", nb.Entities[0]["hop"])
	}
}
