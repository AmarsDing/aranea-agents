package data

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// skipIfPGUnreachable skips the test when the test Postgres is not reachable
// (testhelper.SetupTestPGRaw fatals instead of skipping).
func skipIfPGUnreachable(t *testing.T) {
	t.Helper()
	db, err := sql.Open("postgres", testhelper.TestPGDSN())
	if err != nil {
		t.Skipf("open test postgres: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("test postgres unreachable: %v", err)
	}
}

func setupCacheHitTestRepo(t *testing.T) *usageRepo {
	t.Helper()
	skipIfPGUnreachable(t)
	rawDB := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := rawDB.ExecContext(ctx, `CREATE TABLE model_token_usage_events (
	  id TEXT PRIMARY KEY,
	  occurred_at TEXT NOT NULL,
	  provider_code TEXT NOT NULL DEFAULT '',
	  model_api_id TEXT NOT NULL DEFAULT '',
	  agent_key TEXT NOT NULL DEFAULT '',
	  usage_kind TEXT NOT NULL DEFAULT '',
	  input_tokens INTEGER NOT NULL DEFAULT 0,
	  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
	  created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create model_token_usage_events: %v", err)
	}
	d := &Data{
		rawDB:   rawDB,
		readDB:  rawDB,
		rwDB:    NewReadWriteDB(rawDB, rawDB),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return &usageRepo{data: d}
}

func insertCacheHitRow(t *testing.T, r *usageRepo, id, occurredAt, provider, model, agentKey, kind string, promptTok, cachedTok int64) {
	t.Helper()
	if _, err := r.data.RWDB().WriteDB(context.Background()).ExecContext(context.Background(),
		`INSERT INTO model_token_usage_events (id, occurred_at, provider_code, model_api_id, agent_key, usage_kind, input_tokens, cached_input_tokens, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $2)`,
		id, occurredAt, provider, model, agentKey, kind, promptTok, cachedTok); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func TestCacheHitRatioStats_Aggregates(t *testing.T) {
	r := setupCacheHitTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	inWindow := now.Add(-10 * time.Minute).Format(time.RFC3339)
	inWindow2 := now.Add(-20 * time.Minute).Format(time.RFC3339)
	outside := now.Add(-2 * time.Hour).Format(time.RFC3339)

	// Group A (deepseek/deepseek-chat/agent-a): two in-window samples.
	// weighted = (1000+2000)/(2000+2000) = 0.75; per-turn ratios 0.5 and 1.0
	// -> percentile_cont(0.5) interpolates to 0.75.
	insertCacheHitRow(t, r, "a1", inWindow, "deepseek", "deepseek-chat", "agent-a", "chat_turn", 2000, 1000)
	insertCacheHitRow(t, r, "a2", inWindow2, "deepseek", "deepseek-chat", "agent-a", "chat_turn", 2000, 2000)
	// Below the provider minimum cacheable prompt length: excluded.
	insertCacheHitRow(t, r, "a3", inWindow, "deepseek", "deepseek-chat", "agent-a", "chat_turn", 500, 0)
	// Outside the window: excluded.
	insertCacheHitRow(t, r, "a4", outside, "deepseek", "deepseek-chat", "agent-a", "chat_turn", 4000, 4000)
	// Group B (openai/gpt-4o/agent-b): single sample, exactly at the boundary.
	insertCacheHitRow(t, r, "b1", inWindow, "openai", "gpt-4o", "agent-b", "chat_turn", 1024, 512)
	// team_turn rows are run-level reconciliation only: excluded from aggregates.
	insertCacheHitRow(t, r, "t1", inWindow, "openai", "gpt-4o", "agent-b", "team_turn", 99999, 99999)

	stats, err := r.CacheHitRatioStats(ctx, time.Hour)
	if err != nil {
		t.Fatalf("CacheHitRatioStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2: %+v", len(stats), stats)
	}

	// ORDER BY provider, model, agent_key: deepseek group first.
	a := stats[0]
	if a.Provider != "deepseek" || a.Model != "deepseek-chat" || a.AgentKey != "agent-a" {
		t.Errorf("group A key = %s/%s/%s, want deepseek/deepseek-chat/agent-a", a.Provider, a.Model, a.AgentKey)
	}
	if a.Samples != 2 {
		t.Errorf("group A Samples = %d, want 2 (sub-1024 and out-of-window rows excluded)", a.Samples)
	}
	if a.PromptTok != 4000 || a.CachedTok != 3000 {
		t.Errorf("group A tokens = %d/%d, want 4000/3000", a.PromptTok, a.CachedTok)
	}
	if math.Abs(a.WeightedRatio-0.75) > 1e-9 {
		t.Errorf("group A WeightedRatio = %v, want 0.75", a.WeightedRatio)
	}
	if math.Abs(a.P50Ratio-0.75) > 1e-9 {
		t.Errorf("group A P50Ratio = %v, want 0.75 (interpolated)", a.P50Ratio)
	}

	b := stats[1]
	if b.Provider != "openai" || b.Model != "gpt-4o" || b.AgentKey != "agent-b" {
		t.Errorf("group B key = %s/%s/%s, want openai/gpt-4o/agent-b", b.Provider, b.Model, b.AgentKey)
	}
	if b.Samples != 1 {
		t.Errorf("group B Samples = %d, want 1 (team_turn excluded)", b.Samples)
	}
	if math.Abs(b.WeightedRatio-0.5) > 1e-9 {
		t.Errorf("group B WeightedRatio = %v, want 0.5", b.WeightedRatio)
	}
	if math.Abs(b.P50Ratio-0.5) > 1e-9 {
		t.Errorf("group B P50Ratio = %v, want 0.5", b.P50Ratio)
	}
}

func TestCacheHitRatioStats_Empty(t *testing.T) {
	r := setupCacheHitTestRepo(t)
	ctx := context.Background()
	inWindow := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	// Only sub-threshold rows: everything filtered out.
	insertCacheHitRow(t, r, "x1", inWindow, "deepseek", "deepseek-chat", "agent-a", "chat_turn", 100, 0)

	stats, err := r.CacheHitRatioStats(ctx, time.Hour)
	if err != nil {
		t.Fatalf("CacheHitRatioStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("len(stats) = %d, want 0 (all rows below prompt threshold)", len(stats))
	}

	// Zero window degenerates to the default 1h; a tiny positive window
	// matches nothing.
	stats, err = r.CacheHitRatioStats(ctx, time.Nanosecond)
	if err != nil {
		t.Fatalf("CacheHitRatioStats tiny window: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("len(stats) tiny window = %d, want 0", len(stats))
	}
}
