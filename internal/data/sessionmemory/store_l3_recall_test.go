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

func openL3RecallTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano)
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
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []struct {
		id, stmt string
		imp      float64
		updated  string
	}{
		{"f-dark", "User prefers dark mode for all apps", 0.9, now},
		{"f-tea", "User drinks tea every morning", 0.95, old},
	} {
		if _, err := client.ExecContext(ctx, `
INSERT INTO memory_facts (id, scope_type, scope_id, user_id, agent_id, statement, statement_normalized, fingerprint, importance, status, created_at, updated_at)
VALUES (?, 'agent', 'agent-1', 'user-1', 'agent-1', ?, ?, ?, ?, 'active', ?, ?)`,
			f.id, f.stmt, f.stmt, f.id, f.imp, f.updated, f.updated); err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(client)
}

func TestRecallL3Facts_KeywordRerank(t *testing.T) {
	store := openL3RecallTestStore(t)
	ctx := context.Background()
	rows, err := store.RecallL3Facts(ctx, "agent", "agent-1", "user-1", "dark mode", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if got := m["id"]; got != "f-dark" {
		t.Fatalf("expected f-dark, got %v", got)
	}
}
