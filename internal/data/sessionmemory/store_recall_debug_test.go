package sessionmemory

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openCompositeRecallTestStore(t *testing.T) (*Store, *ent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
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
		`CREATE TABLE IF NOT EXISTS memory_episodes (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL, agent_id TEXT NOT NULL DEFAULT '', episode_kind TEXT NOT NULL DEFAULT 'task',
 title TEXT NOT NULL, outcome_summary TEXT NOT NULL DEFAULT '', importance REAL NOT NULL DEFAULT 0.5,
 consolidation_status TEXT NOT NULL DEFAULT 'consolidated', consolidated_l3_count INTEGER NOT NULL DEFAULT 1,
 metadata_json TEXT NOT NULL DEFAULT '{}', ended_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL DEFAULT '', embedding_blob BLOB, deleted_at TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.ExecContext(ctx, `
INSERT INTO memory_facts (id, scope_type, scope_id, user_id, agent_id, statement, statement_normalized, fingerprint, importance, status, created_at, updated_at)
VALUES ('f-tea', 'agent', 'agent-1', 'user-1', 'agent-1', 'User drinks tea every morning', 'user drinks tea every morning', 'f-tea', 0.4, 'active', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExecContext(ctx, `
INSERT INTO memory_episodes (id, session_id, agent_id, title, outcome_summary, importance, consolidation_status, consolidated_l3_count, created_at, ended_at, updated_at)
VALUES ('ep-dark', 'sess-1', 'agent-1', 'Dark mode preference', 'User prefers dark mode UI', 0.95, 'consolidated', 1, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	return NewStore(client), client
}

func TestRecallL2EpisodesScored_ReturnsNonZeroTotals(t *testing.T) {
	store := openL2RecallTestStore(t)
	ctx := context.Background()
	rows, err := store.RecallL2EpisodesScored(ctx, "agent-1", "", "dark mode", nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected scored rows")
	}
	if rows[0].Scores.Total <= 0 {
		t.Fatalf("expected positive total score, got %.4f", rows[0].Scores.Total)
	}
	if len(rows) > 1 && rows[0].Scores.Total < rows[1].Scores.Total {
		t.Fatalf("expected descending scores, got %.4f then %.4f", rows[0].Scores.Total, rows[1].Scores.Total)
	}
}

func TestCompositeSearchMemories_SortsByTotalScore(t *testing.T) {
	store, _ := openCompositeRecallTestStore(t)
	ctx := context.Background()
	rows, err := store.CompositeSearchMemories(ctx, "agent-1", "", "user-1", "dark mode", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(rows))
	}
	if rows[0].Scores.Total <= rows[1].Scores.Total {
		t.Fatalf("expected descending total scores, got %.4f then %.4f", rows[0].Scores.Total, rows[1].Scores.Total)
	}
}
