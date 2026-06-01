package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openL2RecallTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS memory_episodes (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL, agent_id TEXT NOT NULL DEFAULT '', episode_kind TEXT NOT NULL DEFAULT 'task',
 title TEXT NOT NULL, outcome_summary TEXT NOT NULL DEFAULT '', importance REAL NOT NULL DEFAULT 0.5,
 consolidation_status TEXT NOT NULL DEFAULT 'consolidated', consolidated_l3_count INTEGER NOT NULL DEFAULT 1,
 metadata_json TEXT NOT NULL DEFAULT '{}', ended_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL DEFAULT '', embedding_blob BLOB, deleted_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS memory_action_log (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
 source_event_ids_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL)`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, ep := range []struct {
		id, title, summary string
		imp                float64
		created            string
	}{
		{"ep-dark", "Dark mode preference", "User prefers dark mode UI", 0.9, now},
		{"ep-tea", "Morning tea habit", "User drinks tea every morning", 0.8, old},
	} {
		if _, err := client.ExecContext(ctx, `
INSERT INTO memory_episodes (id, session_id, agent_id, title, outcome_summary, importance, consolidation_status, consolidated_l3_count, created_at, ended_at, updated_at)
VALUES (?, 'sess-1', 'agent-1', ?, ?, ?, 'consolidated', 1, ?, ?, ?)`,
			ep.id, ep.title, ep.summary, ep.imp, ep.created, ep.created, ep.created); err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(client, loggateway.NewNoop())
}

func TestRecallL2Episodes_KeywordRerank(t *testing.T) {
	store := openL2RecallTestStore(t)
	ctx := context.Background()
	rows, err := store.RecallL2Episodes(ctx, "agent-1", "", "dark mode", nil, 1)
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
	if got := m["id"]; got != "ep-dark" {
		t.Fatalf("expected ep-dark, got %v", got)
	}
}

func TestApplyAllEpisodeImportanceDecay(t *testing.T) {
	store := openL2RecallTestStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	n, err := store.ApplyAllEpisodeImportanceDecay(ctx, cutoff, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 old episode decayed, got %d", n)
	}
}
