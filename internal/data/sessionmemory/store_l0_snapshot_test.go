package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openL0SnapshotTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	_, err = client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS memory_l0_assembly_snapshots (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
 span_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '',
 provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
 context_window_tokens INTEGER NOT NULL DEFAULT 0, budget_tokens INTEGER NOT NULL DEFAULT 0,
 recent_window_turns INTEGER NOT NULL DEFAULT 0, recent_window_tokens INTEGER NOT NULL DEFAULT 0,
 summary_token_estimate INTEGER NOT NULL DEFAULT 0,
 l1_field_count INTEGER NOT NULL DEFAULT 0, l1_token_estimate INTEGER NOT NULL DEFAULT 0,
 l3_chunk_count INTEGER NOT NULL DEFAULT 0, l3_token_estimate INTEGER NOT NULL DEFAULT 0,
 l4_path_count INTEGER NOT NULL DEFAULT 0, l4_token_estimate INTEGER NOT NULL DEFAULT 0,
 prompt_token_estimate INTEGER NOT NULL DEFAULT 0, prompt_token_actual INTEGER NOT NULL DEFAULT 0,
 used_ratio REAL NOT NULL DEFAULT 0, truncate_strategy TEXT NOT NULL DEFAULT '',
 truncated_message_count INTEGER NOT NULL DEFAULT 0, summarized_turn_from INTEGER NOT NULL DEFAULT 0,
 summarized_turn_to INTEGER NOT NULL DEFAULT 0, segments_json TEXT NOT NULL DEFAULT '[]',
 warning_codes_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(client, loggateway.NewNoop())
}

func TestInsertAndUpdateL0AssemblySnapshot(t *testing.T) {
	store := openL0SnapshotTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := "snap-1"
	err := store.InsertL0AssemblySnapshot(ctx, biz.L0AssemblySnapshotInsert{
		ID:                  id,
		SessionID:           "sess-1",
		RunID:               "run-1",
		AgentID:             "agent-1",
		Provider:            "openai",
		Model:               "gpt-4",
		ContextWindowTokens: 128000,
		PromptTokenEstimate: 80000,
		UsedRatio:           0.625,
		SegmentsJSON:        `[{"section":"user.input"}]`,
		WarningCodesJSON:    `["near_limit"]`,
		CreatedAt:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateL0SnapshotActual(ctx, id, 90000, 128000); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListL0SnapshotRows(ctx, "sess-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %d", len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if int(m["prompt_token_actual"].(float64)) != 90000 {
		t.Fatalf("actual: %#v", m["prompt_token_actual"])
	}
	ratio := m["used_ratio"].(float64)
	if ratio < 0.70 || ratio > 0.71 {
		t.Fatalf("used_ratio: %v", ratio)
	}
	if warns, _ := m["warning_codes_json"].(string); warns != `["near_limit"]` {
		t.Fatalf("warning_codes_json: %q", warns)
	}
}
