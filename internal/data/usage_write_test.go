package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestRecordTokenUsageEventUpdatesSessionLastModelFields(t *testing.T) {
	rawDB := testhelper.SetupTestPGRaw(t)

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
		  wait_ms INTEGER NOT NULL DEFAULT 0,
		  model_latency_ms INTEGER NOT NULL DEFAULT 0,
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

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, rawDB)))
	defer client.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := client.ExecContext(ctx,
		`INSERT INTO sessions(id, workspace_id, user_id, agent_id, created_at, updated_at)
		 VALUES ($1, '', '', 'agent-1', $2, $3)`,
		"sess-usage-1", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := &usageRepo{data: &Data{entClient: client, rw: NewReadWriteClient(client, client)}}
	ev := biz.TokenUsageEvent{
		ID:             "usage-ev-1",
		SessionID:      "sess-usage-1",
		AgentID:        "agent-1",
		AgentKey:       "test-agent",
		ProviderCode:   "openai",
		ModelAPIID:     "gpt-4o-mini",
		InputTokens:    10,
		OutputTokens:   5,
		TotalTokens:    15,
		CallCount:      1,
		Status:         "success",
		UsageKind:      biz.UsageKindChatTurn,
		WaitMS:         300000,
		ModelLatencyMS: 20000,
		LatencyMS:      320000,
		MetadataJSON:   "{}",
		OccurredAt:     now,
		DateKey:        time.Now().UTC().Format("2006-01-02"),
		HourKey:        time.Now().UTC().Format("2006-01-02T15"),
		CreatedAt:      now,
	}
	if _, err := repo.RecordTokenUsageEvent(ctx, ev); err != nil {
		t.Fatalf("RecordTokenUsageEvent: %v", err)
	}

	var count int
	err = entQueryRowScan(client, ctx,
		`SELECT COUNT(*) FROM model_token_usage_events WHERE id = $1`,
		[]any{"usage-ev-1"},
		&count,
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 usage event row, got %d", count)
	}
	var waitMS, modelMS int
	err = entQueryRowScan(client, ctx,
		`SELECT wait_ms, model_latency_ms FROM model_token_usage_events WHERE id = $1`,
		[]any{"usage-ev-1"},
		&waitMS, &modelMS,
	)
	if err != nil {
		t.Fatal(err)
	}
	if waitMS != 300000 || modelMS != 20000 {
		t.Fatalf("wait_ms=%d model_latency_ms=%d", waitMS, modelMS)
	}
}

func TestPurgeUsageEventsOlderThanCleansRollups(t *testing.T) {
	rawDB := testhelper.SetupTestPGRaw(t)

	ctx := context.Background()
	for _, stmt := range []string{
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
		  wait_ms INTEGER NOT NULL DEFAULT 0,
		  model_latency_ms INTEGER NOT NULL DEFAULT 0,
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

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, rawDB)))
	defer client.Close()

	repo := &usageRepo{data: &Data{entClient: client, rw: NewReadWriteClient(client, client)}}

	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -10).Format("2006-01-02")
	oldHour := now.AddDate(0, 0, -10).Format("2006-01-02T15")
	recentDate := now.Format("2006-01-02")
	recentHour := now.Format("2006-01-02T15")
	occurred := now.Format(time.RFC3339)

	insertEvent := func(id, dateKey, hourKey string) {
		_, err := client.ExecContext(ctx,
			`INSERT INTO model_token_usage_events(
				id, occurred_at, date_key, hour_key, agent_id, provider_code, model_api_id,
				usage_kind, call_count, input_tokens, output_tokens, total_tokens, status, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, 1, 1, 2, 'success', $9)`,
			id, occurred, dateKey, hourKey, "agent-1", "openai", "gpt-4o-mini", biz.UsageKindChatTurn, occurred,
		)
		if err != nil {
			t.Fatalf("insert event %s: %v", id, err)
		}
	}
	insertDaily := func(id, dateKey string) {
		_, err := client.ExecContext(ctx,
			`INSERT INTO model_token_usage_daily(
				id, date_key, agent_id, provider_code, model_api_id, usage_kind,
				call_count, request_count, success_count, input_tokens, total_tokens, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 1, 1, 1, 1, 2, $7, $8)`,
			id, dateKey, "agent-1", "openai", "gpt-4o-mini", biz.UsageKindChatTurn, occurred, occurred,
		)
		if err != nil {
			t.Fatalf("insert daily %s: %v", id, err)
		}
	}
	insertHourly := func(id, hourKey string) {
		_, err := client.ExecContext(ctx,
			`INSERT INTO model_token_usage_hourly(
				id, hour_key, agent_id, provider_code, model_api_id, usage_kind,
				call_count, request_count, success_count, input_tokens, total_tokens, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 1, 1, 1, 1, 2, $7, $8)`,
			id, hourKey, "agent-1", "openai", "gpt-4o-mini", biz.UsageKindChatTurn, occurred, occurred,
		)
		if err != nil {
			t.Fatalf("insert hourly %s: %v", id, err)
		}
	}

	insertEvent("old-event", oldDate, oldHour)
	insertDaily("old-daily", oldDate)
	insertHourly("old-hourly", oldHour)
	insertEvent("recent-event", recentDate, recentHour)
	insertDaily("recent-daily", recentDate)
	insertHourly("recent-hourly", recentHour)

	deleted, err := repo.PurgeUsageEventsOlderThan(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeUsageEventsOlderThan: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted event, got %d", deleted)
	}

	count := func(table, where string, args ...any) int {
		var n int
		if err := entQueryRowScan(client, ctx,
			`SELECT COUNT(*) FROM `+table+` WHERE `+where,
			args,
			&n,
		); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}

	if got := count("model_token_usage_events", "id = $1", "old-event"); got != 0 {
		t.Fatalf("expected old event deleted, got %d", got)
	}
	if got := count("model_token_usage_events", "id = $1", "recent-event"); got != 1 {
		t.Fatalf("expected recent event retained, got %d", got)
	}
	if got := count("model_token_usage_daily", "id = $1", "old-daily"); got != 0 {
		t.Fatalf("expected old daily rollup deleted, got %d", got)
	}
	if got := count("model_token_usage_daily", "id = $1", "recent-daily"); got != 1 {
		t.Fatalf("expected recent daily rollup retained, got %d", got)
	}
	if got := count("model_token_usage_hourly", "id = $1", "old-hourly"); got != 0 {
		t.Fatalf("expected old hourly rollup deleted, got %d", got)
	}
	if got := count("model_token_usage_hourly", "id = $1", "recent-hourly"); got != 1 {
		t.Fatalf("expected recent hourly rollup retained, got %d", got)
	}
}

func TestDDLUsageWaitMSColumnsBackfill(t *testing.T) {
	rawDB := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := rawDB.ExecContext(ctx, `CREATE TABLE model_token_usage_events (
		id TEXT PRIMARY KEY,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.ExecContext(ctx,
		`INSERT INTO model_token_usage_events(id, metadata_json) VALUES ($1, $2)`,
		"u1", `{"wait_ms":300000,"model_latency_ms":20000}`); err != nil {
		t.Fatal(err)
	}
	if err := ddlUsageWaitMSColumns(ctx, rawDB, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ddlUsageWaitMSColumns: %v", err)
	}
	if err := ddlUsageWaitMSColumns(ctx, rawDB, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("idempotent re-run: %v", err)
	}
	var waitMS, modelMS int
	if err := rawDB.QueryRowContext(ctx,
		`SELECT wait_ms, model_latency_ms FROM model_token_usage_events WHERE id = $1`, "u1",
	).Scan(&waitMS, &modelMS); err != nil {
		t.Fatal(err)
	}
	if waitMS != 300000 || modelMS != 20000 {
		t.Fatalf("backfill wait_ms=%d model_latency_ms=%d", waitMS, modelMS)
	}
}
