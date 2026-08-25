package data

import (
	"context"
	"math"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupRunCacheHitTestRepo builds the minimal two-table shape the run-level
// query touches: usage events (message_id / metadata_json included) plus
// team_run_steps for the fallback branch's step-ownership linkage.
func setupRunCacheHitTestRepo(t *testing.T) *usageRepo {
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
	  message_id TEXT NOT NULL DEFAULT '',
	  input_tokens INTEGER NOT NULL DEFAULT 0,
	  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
	  metadata_json TEXT,
	  created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create model_token_usage_events: %v", err)
	}
	if _, err := rawDB.ExecContext(ctx, `CREATE TABLE team_run_steps (
	  id TEXT PRIMARY KEY,
	  run_id TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create team_run_steps: %v", err)
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

// metaJSON 传 nil 落 NULL，传字符串落 TEXT——覆盖 genuine 行的 NULL/无键两形态。
func insertRunUsageRow(t *testing.T, r *usageRepo, id, kind, messageID string, metaJSON any, promptTok, cachedTok int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO model_token_usage_events (id, occurred_at, usage_kind, message_id, input_tokens, cached_input_tokens, metadata_json, created_at)
		 VALUES ($1, '2026-08-25T00:00:00Z', $2, $3, $4, $5, $6, '2026-08-25T00:00:00Z')`,
		id, kind, messageID, promptTok, cachedTok, metaJSON); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func insertRunStep(t *testing.T, r *usageRepo, stepID, runID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO team_run_steps (id, run_id) VALUES ($1, $2)`, stepID, runID); err != nil {
		t.Fatalf("insert step %s: %v", stepID, err)
	}
}

// 主分支：team_turn 对账行（message_id = run id）直取 run 总账。
func TestRunCacheHitRatio_TeamTurnRow(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	insertRunUsageRow(t, r, "tt1", "team_turn", "run-a", nil, 4000, 3000)

	hit, err := r.RunCacheHitRatio(context.Background(), "run-a")
	if err != nil {
		t.Fatalf("RunCacheHitRatio: %v", err)
	}
	if !hit.Found {
		t.Fatal("Found = false, want true (team_turn row present)")
	}
	if hit.PromptTok != 4000 || hit.CachedTok != 3000 {
		t.Errorf("tokens = %d/%d, want 4000/3000", hit.PromptTok, hit.CachedTok)
	}
	if math.Abs(hit.Ratio-0.75) > 1e-9 {
		t.Errorf("Ratio = %v, want 0.75", hit.Ratio)
	}
}

// 回退分支：无 team_turn 行（失败/取消 run）→ genuine 成员行按 step 归属求和；
// 镜像行（attribution 非空）与他 run 成员行必须排除。
func TestRunCacheHitRatio_FallbackGenuineMemberRows(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	insertRunStep(t, r, "s1", "run-f")
	insertRunStep(t, r, "s2", "run-f")
	insertRunStep(t, r, "s9", "run-other")

	// genuine 行：无 metadata / 空 attribution / 显式空串 attribution 三种形态都算。
	insertRunUsageRow(t, r, "m1", "team_member", "s1", nil, 1000, 600)
	insertRunUsageRow(t, r, "m2", "team_member", "s2", `{"source":"team_member_step"}`, 2000, 1400)
	// 镜像行：与 team_turn 总账同额，排除（防双计）。
	insertRunUsageRow(t, r, "m3", "team_member", "s1", `{"usage_attribution":"member_level_stream"}`, 50000, 50000)
	insertRunUsageRow(t, r, "m4", "team_member", "s1", `{"usage_attribution":"run_level_anchor_fallback"}`, 50000, 50000)
	// 他 run 的 genuine 行：排除。
	insertRunUsageRow(t, r, "m5", "team_member", "s9", nil, 7777, 7777)
	// chat_turn 行不算 team_member：排除。
	insertRunUsageRow(t, r, "m6", "chat_turn", "s1", nil, 9999, 9999)

	hit, err := r.RunCacheHitRatio(context.Background(), "run-f")
	if err != nil {
		t.Fatalf("RunCacheHitRatio: %v", err)
	}
	if !hit.Found {
		t.Fatal("Found = false, want true (genuine member rows present)")
	}
	if hit.PromptTok != 3000 || hit.CachedTok != 2000 {
		t.Errorf("tokens = %d/%d, want 3000/2000 (mirror + other-run + chat_turn excluded)", hit.PromptTok, hit.CachedTok)
	}
	if math.Abs(hit.Ratio-2.0/3.0) > 1e-9 {
		t.Errorf("Ratio = %v, want 0.6667", hit.Ratio)
	}
}

// 无任何 usage 行的 run：Found=false（调用方区分"无数据"与真实 0%）。
func TestRunCacheHitRatio_NotFound(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)

	hit, err := r.RunCacheHitRatio(context.Background(), "run-missing")
	if err != nil {
		t.Fatalf("RunCacheHitRatio: %v", err)
	}
	if hit.Found {
		t.Errorf("Found = true, want false for unknown run: %+v", hit)
	}

	// 空 runID 直接短路，不触库。
	hit, err = r.RunCacheHitRatio(context.Background(), "  ")
	if err != nil {
		t.Fatalf("RunCacheHitRatio empty id: %v", err)
	}
	if hit.Found {
		t.Errorf("Found = true, want false for blank run id")
	}
}
