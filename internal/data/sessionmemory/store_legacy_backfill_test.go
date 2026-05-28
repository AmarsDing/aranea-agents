package sessionmemory_test

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/sessionmemory"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func TestBackfillLegacyTRPCMemoryEntities_migratesAndIdempotent(t *testing.T) {
	store := openLegacyBackfillStore(t)
	ctx := context.Background()

	insertLegacyEntity(t, store, "leg-1", "agent-1", "user-1", "Alice prefers tea", `{"topics":["pref"]}`, 0.8)

	n, skipped, err := store.BackfillLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 migrated, got %d", n)
	}
	if skipped != 0 {
		t.Fatalf("expected 0 skipped, got %d", skipped)
	}

	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-1", "", "active", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 fact, got total=%d rows=%d", total, len(rows))
	}

	pending, err := store.CountPendingLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("expected 0 pending legacy entities, got %d", pending)
	}

	n, skipped, err = store.BackfillLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 on idempotent run, got %d", n)
	}
	if skipped != 0 {
		t.Fatalf("expected 0 skipped on idempotent run, got %d", skipped)
	}
}

func TestBackfillLegacyTRPCMemoryEntities_skipsInvalidRows(t *testing.T) {
	store := openLegacyBackfillStore(t)
	ctx := context.Background()

	insertLegacyEntity(t, store, "leg-invalid", "", "user-1", "", "{}", 0.5)

	n, skipped, err := store.BackfillLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 migrated, got %d", n)
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", skipped)
	}
	pending, err := store.CountPendingLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("expected 0 pending after skip, got %d", pending)
	}
}

func openLegacyBackfillStore(t *testing.T) *sessionmemory.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS memory_facts (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '',
 statement TEXT NOT NULL, statement_normalized TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', details_markdown TEXT NOT NULL DEFAULT '',
 fact_kind TEXT NOT NULL DEFAULT 'fact', tags_json TEXT NOT NULL DEFAULT '[]',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 use_count INTEGER NOT NULL DEFAULT 0, hit_count INTEGER NOT NULL DEFAULT 0,
 positive_feedback_count INTEGER NOT NULL DEFAULT 0, negative_feedback_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
 source_kind TEXT NOT NULL DEFAULT 'manual', source_episode_id TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', source_external TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active', superseded_by TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BLOB, embedding_norm REAL NOT NULL DEFAULT 0,
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '', pii_types TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', quality_score REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, fingerprint))`,
		`CREATE TABLE IF NOT EXISTS memory_action_log (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
 source_event_ids_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL)`,
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
		`CREATE TABLE IF NOT EXISTS schema_migrations (
 version INTEGER PRIMARY KEY NOT NULL,
 name TEXT NOT NULL,
 applied_at TEXT NOT NULL)`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return sessionmemory.NewStore(client)
}

func insertLegacyEntity(t *testing.T, store *sessionmemory.Store, id, scopeID, userID, desc, meta string, importance float64) {
	t.Helper()
	ctx := context.Background()
	client := store.Client()
	if client == nil {
		t.Fatal("store client nil")
	}
	_, err := client.ExecContext(ctx, `
INSERT INTO memory_entities (
 id, scope_type, scope_id, workspace_id, user_id,
 entity_type, name, name_normalized, aliases_json, description, attributes_json,
 importance, confidence, use_count, source_kind,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 status, merged_into, metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, "trpc_memory", scopeID, "", userID,
		"memory_fact", desc, "legacy", "[]", desc, "{}",
		importance, 0.85, 0, "legacy",
		"pending", "", 0, nil, 0.0,
		"active", "", meta, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
}
