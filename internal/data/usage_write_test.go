package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/glebarez/go-sqlite/compat"
)

func TestRecordTokenUsageEventUpdatesSessionLastModelFields(t *testing.T) {
	rawDB, err := sql.Open("sqlite", "file:usage_write_test?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	if _, err := rawDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE sessions (
		  id TEXT PRIMARY KEY,
		  workspace_id TEXT NOT NULL DEFAULT '',
		  user_id TEXT NOT NULL DEFAULT '',
		  agent_id TEXT NOT NULL DEFAULT '',
		  model_call_count INTEGER NOT NULL DEFAULT 0,
		  input_tokens INTEGER NOT NULL DEFAULT 0,
		  output_tokens INTEGER NOT NULL DEFAULT 0,
		  total_tokens INTEGER NOT NULL DEFAULT 0,
		  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  last_provider TEXT NOT NULL DEFAULT '',
		  last_model TEXT NOT NULL DEFAULT '',
		  created_at TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL DEFAULT '',
		  deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE model_token_usage_events (
		  id TEXT PRIMARY KEY,
		  occurred_at TEXT NOT NULL,
		  date_key TEXT NOT NULL,
		  hour_key TEXT NOT NULL,
		  workspace_id TEXT NOT NULL DEFAULT '',
		  user_id TEXT NOT NULL DEFAULT '',
		  team_id TEXT NOT NULL DEFAULT '',
		  agent_id TEXT NOT NULL DEFAULT '',
		  agent_key TEXT NOT NULL DEFAULT '',
		  session_id TEXT NOT NULL DEFAULT '',
		  message_id TEXT NOT NULL DEFAULT '',
		  request_id TEXT NOT NULL DEFAULT '',
		  provider_code TEXT NOT NULL DEFAULT '',
		  canonical_provider_code TEXT NOT NULL DEFAULT '',
		  provider_type TEXT NOT NULL DEFAULT '',
		  provider_display_name TEXT NOT NULL DEFAULT '',
		  model_api_id TEXT NOT NULL DEFAULT '',
		  model_display_name TEXT NOT NULL DEFAULT '',
		  model_category_json TEXT NOT NULL DEFAULT '[]',
		  usage_kind TEXT NOT NULL DEFAULT 'chat',
		  call_count INTEGER NOT NULL DEFAULT 1,
		  input_tokens INTEGER NOT NULL DEFAULT 0,
		  output_tokens INTEGER NOT NULL DEFAULT 0,
		  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
		  cache_write_tokens INTEGER NOT NULL DEFAULT 0,
		  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		  embedding_tokens INTEGER NOT NULL DEFAULT 0,
		  total_tokens INTEGER NOT NULL DEFAULT 0,
		  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  cache_write_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
		  input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  output_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  cached_input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  cache_write_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  reasoning_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  embedding_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  latency_ms INTEGER NOT NULL DEFAULT 0,
		  time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
		  tokens_per_second REAL NOT NULL DEFAULT 0,
		  status TEXT NOT NULL DEFAULT 'success',
		  error_code TEXT NOT NULL DEFAULT '',
		  error_message TEXT NOT NULL DEFAULT '',
		  retry_count INTEGER NOT NULL DEFAULT 0,
		  prompt_mode TEXT NOT NULL DEFAULT '',
		  max_output_tokens INTEGER NOT NULL DEFAULT 0,
		  context_window_k INTEGER NOT NULL DEFAULT 0,
		  stream_enabled INTEGER NOT NULL DEFAULT 0,
		  metadata_json TEXT NOT NULL DEFAULT '{}',
		  created_at TEXT NOT NULL
		)`,
		`CREATE TABLE model_token_usage_daily (
		  id TEXT PRIMARY KEY,
		  date_key TEXT NOT NULL,
		  workspace_id TEXT NOT NULL DEFAULT '',
		  agent_id TEXT NOT NULL DEFAULT '',
		  agent_key TEXT NOT NULL DEFAULT '',
		  provider_code TEXT NOT NULL DEFAULT '',
		  model_api_id TEXT NOT NULL DEFAULT '',
		  usage_kind TEXT NOT NULL DEFAULT 'chat',
		  call_count INTEGER NOT NULL DEFAULT 0,
		  request_count INTEGER NOT NULL DEFAULT 0,
		  success_count INTEGER NOT NULL DEFAULT 0,
		  failed_count INTEGER NOT NULL DEFAULT 0,
		  cancelled_count INTEGER NOT NULL DEFAULT 0,
		  input_tokens INTEGER NOT NULL DEFAULT 0,
		  output_tokens INTEGER NOT NULL DEFAULT 0,
		  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
		  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		  embedding_tokens INTEGER NOT NULL DEFAULT 0,
		  total_tokens INTEGER NOT NULL DEFAULT 0,
		  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  avg_latency_ms REAL NOT NULL DEFAULT 0,
		  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
		  created_at TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL DEFAULT '',
		  UNIQUE(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
		)`,
		`CREATE TABLE model_token_usage_hourly (
		  id TEXT PRIMARY KEY,
		  hour_key TEXT NOT NULL,
		  workspace_id TEXT NOT NULL DEFAULT '',
		  agent_id TEXT NOT NULL DEFAULT '',
		  agent_key TEXT NOT NULL DEFAULT '',
		  provider_code TEXT NOT NULL DEFAULT '',
		  model_api_id TEXT NOT NULL DEFAULT '',
		  usage_kind TEXT NOT NULL DEFAULT 'chat',
		  call_count INTEGER NOT NULL DEFAULT 0,
		  request_count INTEGER NOT NULL DEFAULT 0,
		  success_count INTEGER NOT NULL DEFAULT 0,
		  failed_count INTEGER NOT NULL DEFAULT 0,
		  cancelled_count INTEGER NOT NULL DEFAULT 0,
		  input_tokens INTEGER NOT NULL DEFAULT 0,
		  output_tokens INTEGER NOT NULL DEFAULT 0,
		  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
		  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		  embedding_tokens INTEGER NOT NULL DEFAULT 0,
		  total_tokens INTEGER NOT NULL DEFAULT 0,
		  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
		  avg_latency_ms REAL NOT NULL DEFAULT 0,
		  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
		  created_at TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL DEFAULT '',
		  UNIQUE(hour_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
		)`,
	} {
		if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, rawDB)))
	defer client.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = client.ExecContext(ctx,
		`INSERT INTO sessions(id, workspace_id, user_id, agent_id, created_at, updated_at)
		 VALUES (?, '', '', 'agent-1', ?, ?)`,
		"sess-usage-1", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := &usageRepo{data: &Data{entClient: client}}
	ev := biz.TokenUsageEvent{
		ID:           "usage-ev-1",
		SessionID:    "sess-usage-1",
		AgentID:      "agent-1",
		AgentKey:     "test-agent",
		ProviderCode: "openai",
		ModelAPIID:   "gpt-4o-mini",
		InputTokens:  10,
		OutputTokens: 5,
		TotalTokens:  15,
		CallCount:    1,
		Status:       "success",
		UsageKind:    biz.UsageKindChatTurn,
		MetadataJSON: "{}",
		OccurredAt:   now,
		DateKey:      time.Now().UTC().Format("2006-01-02"),
		HourKey:      time.Now().UTC().Format("2006-01-02T15"),
		CreatedAt:    now,
	}
	if _, err := repo.RecordTokenUsageEvent(ctx, ev); err != nil {
		t.Fatalf("RecordTokenUsageEvent: %v", err)
	}

	var count int
	err = entQueryRowScan(client, ctx,
		`SELECT COUNT(*) FROM model_token_usage_events WHERE id = ?`,
		[]any{"usage-ev-1"},
		&count,
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 usage event row, got %d", count)
	}
}
