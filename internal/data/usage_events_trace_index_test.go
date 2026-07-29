package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// The trace_id expression index migration must create the index and keep
// AggregateUsageByTrace correct (per-trace sums + latest provider/model).
func TestDDLUsageEventsTraceIDIndex(t *testing.T) {
	rawDB := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := rawDB.ExecContext(ctx, `CREATE TABLE model_token_usage_events (
	  id TEXT PRIMARY KEY,
	  occurred_at TEXT NOT NULL,
	  provider_code TEXT NOT NULL DEFAULT '',
	  model_api_id TEXT NOT NULL DEFAULT '',
	  total_tokens INTEGER NOT NULL DEFAULT 0,
	  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
	  metadata_json TEXT NOT NULL DEFAULT '{}',
	  created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create model_token_usage_events: %v", err)
	}

	if err := ddlUsageEventsTraceIDIndex(ctx, rawDB, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ddlUsageEventsTraceIDIndex: %v", err)
	}
	exists, err := DialectPostgres.IndexExists(ctx, rawDB, "model_token_usage_events", "idx_model_token_usage_events_trace_id")
	if err != nil {
		t.Fatalf("IndexExists: %v", err)
	}
	if !exists {
		t.Fatal("idx_model_token_usage_events_trace_id not created")
	}
	// Idempotent re-run.
	if err := ddlUsageEventsTraceIDIndex(ctx, rawDB, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ddlUsageEventsTraceIDIndex re-run: %v", err)
	}

	insert := func(id, traceID, provider, model string, tokens, costMicro int64, occurredAt string) {
		t.Helper()
		if _, err := rawDB.ExecContext(ctx,
			`INSERT INTO model_token_usage_events (id, occurred_at, provider_code, model_api_id, total_tokens, total_cost_micro_usd, metadata_json, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $2)`,
			id, occurredAt, provider, model, tokens, costMicro, `{"trace_id":"`+traceID+`"}`); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("e1", "trace-a", "deepseek", "deepseek-chat", 100, 500, "2026-07-29T01:00:00Z")
	insert("e2", "trace-a", "openai", "gpt-4o", 200, 1500, "2026-07-29T02:00:00Z")
	insert("e3", "trace-b", "anthropic", "claude-4", 50, 250, "2026-07-29T01:30:00Z")
	// No trace_id in metadata: must not leak into other traces.
	insert("e4", "", "deepseek", "deepseek-chat", 999, 999, "2026-07-29T03:00:00Z")

	d := &Data{
		rawDB:   rawDB,
		readDB:  rawDB,
		rwDB:    NewReadWriteDB(rawDB, rawDB),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	r := &monitorRepo{data: d}

	agg, err := r.AggregateUsageByTrace(ctx, "trace-a")
	if err != nil {
		t.Fatalf("AggregateUsageByTrace: %v", err)
	}
	if agg.TotalTokens != 300 || agg.CallCount != 2 {
		t.Errorf("trace-a tokens/count = %d/%d, want 300/2", agg.TotalTokens, agg.CallCount)
	}
	if agg.TotalCostUsd != 0.002 {
		t.Errorf("trace-a cost = %v, want 0.002", agg.TotalCostUsd)
	}
	// Provider/model come from the latest event (occurred_at DESC): e2.
	if agg.Provider != "openai" || agg.Model != "gpt-4o" {
		t.Errorf("trace-a provider/model = %q/%q, want openai/gpt-4o", agg.Provider, agg.Model)
	}

	missing, err := r.AggregateUsageByTrace(ctx, "trace-gone")
	if err != nil {
		t.Fatalf("AggregateUsageByTrace missing: %v", err)
	}
	if missing.CallCount != 0 || missing.TotalTokens != 0 || missing.Provider != "" {
		t.Errorf("missing trace agg = %+v, want zeros", missing)
	}
}
