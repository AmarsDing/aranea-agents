package data_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/sessionmemory"
	trpcmem "aranea-agents/internal/memory/trpc"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

func openTestSessionMemoryStore(t *testing.T) *sessionmemory.Store {
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
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
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

func TestSQLiteMemoryService_AddMemoryWritesFactVisibleToAdmin(t *testing.T) {
	store := openTestSessionMemoryStore(t)
	svc := trpcmem.NewSQLiteMemoryService(store, nil, nil, nil, nil)
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.AddMemory(ctx, uk, "My name is Alice", []string{"profile"}); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-1", "", "active", "", 20, 0)
	if err != nil {
		t.Fatalf("ListFactRows: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 fact, got total=%d rows=%d", total, len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if got := m["statement"]; got != "My name is Alice" {
		t.Fatalf("statement=%v", got)
	}
	if got := m["source_kind"]; got != "trpc_memory" {
		t.Fatalf("source_kind=%v", got)
	}
}

func TestSQLiteMemoryService_AddMemoryDedupByFingerprint(t *testing.T) {
	store := openTestSessionMemoryStore(t)
	svc := trpcmem.NewSQLiteMemoryService(store, nil, nil, nil, nil)
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-dedup", UserID: "user-dedup"}
	stmt := "I prefer tea in the morning"
	if err := svc.AddMemory(ctx, uk, stmt, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMemory(ctx, uk, stmt, nil); err != nil {
		t.Fatal(err)
	}
	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-dedup", "", "active", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 deduped fact, got total=%d rows=%d", total, len(rows))
	}
}

func TestSQLiteMemoryService_ReadMemoriesFromFacts(t *testing.T) {
	store := openTestSessionMemoryStore(t)
	svc := trpcmem.NewSQLiteMemoryService(store, nil, nil, nil, nil)
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-2", UserID: "user-2"}
	if err := svc.AddMemory(ctx, uk, "I prefer dark mode", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Memory.Memory != "I prefer dark mode" {
		t.Fatalf("memory=%q", entries[0].Memory.Memory)
	}
}

func TestSQLiteMemoryService_DeleteAndClear(t *testing.T) {
	store := openTestSessionMemoryStore(t)
	svc := trpcmem.NewSQLiteMemoryService(store, nil, nil, nil, nil)
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-3", UserID: "user-3"}
	if err := svc.AddMemory(ctx, uk, "fact one", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read: err=%v len=%d", err, len(entries))
	}
	mk := trpcmemory.Key{AppName: uk.AppName, UserID: uk.UserID, MemoryID: entries[0].ID}
	if err := svc.DeleteMemory(ctx, mk); err != nil {
		t.Fatal(err)
	}
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(entries))
	}
	if err := svc.AddMemory(ctx, uk, "fact two", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearMemories(ctx, uk); err != nil {
		t.Fatal(err)
	}
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 after clear, got %d", len(entries))
	}
}

func TestMemoryAdminUsecase_RequireAdminWhenStoreMissing(t *testing.T) {
	uc := biz.NewMemoryAdminUsecase(nil, &biz.MemoryUsecase{}, nil)
	if uc == nil {
		t.Fatal("expected vec-only usecase")
	}
	_, _, _, _, err := uc.ListFactRows(context.Background(), "agent", "a1", "", "", "", 10, 0)
	if err == nil {
		t.Fatal("expected error when admin store missing")
	}
}
