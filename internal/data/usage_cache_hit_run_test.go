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
  output_tokens INTEGER NOT NULL DEFAULT 0,
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
	insertRunUsageRowFull(t, r, id, kind, messageID, "", metaJSON, promptTok, 0, cachedTok)
}

// insertRunUsageRowFull 是全字段版插入：agentKey/outputTok 供成员聚合与
// completion 回读断言使用；旧 helper 保留原位签名，存量调用零改动。
func insertRunUsageRowFull(t *testing.T, r *usageRepo, id, kind, messageID, agentKey string, metaJSON any, promptTok, outputTok, cachedTok int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO model_token_usage_events (id, occurred_at, usage_kind, message_id, agent_key, input_tokens, output_tokens, cached_input_tokens, metadata_json, created_at)
		 VALUES ($1, '2026-08-25T00:00:00Z', $2, $3, $4, $5, $6, $7, $8, '2026-08-25T00:00:00Z')`,
		id, kind, messageID, agentKey, promptTok, outputTok, cachedTok, metaJSON); err != nil {
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
	insertRunUsageRowFull(t, r, "tt1", "team_turn", "run-a", "", nil, 4000, 800, 3000)

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
	if hit.CompletionTok != 800 {
		t.Errorf("CompletionTok = %d, want 800（R7 stats completion 分列）", hit.CompletionTok)
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

// ─── R7（G-2）RunTurnPeak：成员行 MAX(input_tokens) ───

// 查询对写路径不可知（2026-08-27 口径修正）：genuine 行（旧路径）与
// attribution 标记行（graph 路径，每行=成员 run 总量）都计入峰值——生产
// graph 路径带 token 的 member 行全带标记，排除则峰值恒 0；两类行按 run
// 互斥（写路径保证，见 team/usage_record.go），测试混合两类证明读侧无
// attribution 过滤。team_turn 对账行（另一 usage_kind）与他 run 行排除。
func TestRunTurnPeak_MemberRowsMax(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	insertRunStep(t, r, "s1", "run-p")
	insertRunStep(t, r, "s2", "run-p")
	insertRunStep(t, r, "s9", "run-other")

	insertRunUsageRow(t, r, "m1", "team_member", "s1", nil, 12000, 0)
	insertRunUsageRow(t, r, "m2", "team_member", "s2", nil, 47000, 0)
	// graph 路径标记行：成员 run 总量，计入峰值。
	insertRunUsageRow(t, r, "m3", "team_member", "s1", `{"usage_attribution":"member_level_stream"}`, 9000000, 0)
	// step 归属解析失败时 member 行 keyed 为 run id（与 team_turn 行同约定）：
	// 必须被 message_id=run id 子句捞到。
	insertRunUsageRow(t, r, "m6", "team_member", "run-p", `{"usage_attribution":"run_level_anchor_fallback"}`, 50000, 0)
	// team_turn 对账行：run 总账但属另一 usage_kind，排除。
	insertRunUsageRow(t, r, "m4", "team_turn", "run-p", nil, 99000000, 0)
	// 他 run 的 member 行：排除。
	insertRunUsageRow(t, r, "m5", "team_member", "s9", nil, 888888, 0)

	peak, err := r.RunTurnPeak(context.Background(), "run-p")
	if err != nil {
		t.Fatalf("RunTurnPeak: %v", err)
	}
	if !peak.Found {
		t.Fatal("Found = false, want true")
	}
	if peak.MaxInputTokens != 9000000 {
		t.Errorf("MaxInputTokens = %d, want 9000000（team_turn/他 run 行排除，run-id keyed member 行计入）", peak.MaxInputTokens)
	}
}

// 无 team_member 行的 run：Found=false。team_turn 对账行 message_id=run id
// 恰好命中 message_id 匹配，必须被 usage_kind 过滤挡下（防 message_id=run id
// 子句把对账行泄入成员口径）；调用方据此区分「无数据」与峰值 0。
func TestRunTurnPeak_NotFound(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	insertRunUsageRow(t, r, "m1", "team_turn", "run-p", nil, 5000, 0)

	peak, err := r.RunTurnPeak(context.Background(), "run-p")
	if err != nil {
		t.Fatalf("RunTurnPeak: %v", err)
	}
	if peak.Found {
		t.Errorf("Found = true, want false（team_turn 对账行非成员口径）: %+v", peak)
	}
	peak, err = r.RunTurnPeak(context.Background(), " ")
	if err != nil {
		t.Fatalf("RunTurnPeak blank id: %v", err)
	}
	if peak.Found {
		t.Error("Found = true, want false for blank run id")
	}
}

// ─── R7 RunMemberUsageStats：GROUP BY agent_key 成员聚合 ───

// 成员分桶求和（2026-08-27 口径修正）：genuine 行（旧路径）与 attribution
// 标记行（graph 路径，member_level_stream=成员 run 总量、remainder 归 anchor）
// 都计入——生产 graph 路径带 token 行全带标记，排除则 members 段恒空；两类
// 按 run 互斥（写路径保证），测试混合两类证明读侧无 attribution 过滤。
// 他 run/chat_turn（另一 kind）排除。
func TestRunMemberUsageStats_GroupByAgent(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	insertRunStep(t, r, "s1", "run-m")
	insertRunStep(t, r, "s2", "run-m")
	insertRunStep(t, r, "s3", "run-m")
	insertRunStep(t, r, "s9", "run-other")

	// planner 两次 genuine 调用（跨 step）。
	insertRunUsageRowFull(t, r, "m1", "team_member", "s1", "planner", nil, 1000, 100, 600)
	insertRunUsageRowFull(t, r, "m2", "team_member", "s2", "planner", nil, 2000, 200, 1400)
	// executor 一次。
	insertRunUsageRowFull(t, r, "m3", "team_member", "s3", "executor", nil, 500, 50, 0)
	// graph 路径标记行：planner 的 anchor remainder，计入 planner 桶。
	insertRunUsageRowFull(t, r, "m4", "team_member", "s1", "planner", `{"usage_attribution":"stream_anchor_remainder"}`, 77777, 0, 77777)
	// step 归属解析失败的 member 行 keyed 为 run id：必须被 message_id=run id
	// 子句捞到，计入 executor 桶。
	insertRunUsageRowFull(t, r, "m7", "team_member", "run-m", "executor", `{"usage_attribution":"member_level_stream"}`, 100, 10, 10)
	// 他 run / chat_turn：排除。
	insertRunUsageRowFull(t, r, "m5", "team_member", "s9", "planner", nil, 8888, 0, 8888)
	insertRunUsageRowFull(t, r, "m6", "chat_turn", "s1", "planner", nil, 9999, 0, 9999)

	members, err := r.RunMemberUsageStats(context.Background(), "run-m")
	if err != nil {
		t.Fatalf("RunMemberUsageStats: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2（executor/planner）: %+v", len(members), members)
	}
	// ORDER BY agent_key：executor 在前。
	if members[0].AgentKey != "executor" || members[0].PromptTok != 600 || members[0].CompletionTok != 60 || members[0].CachedTok != 10 || members[0].Calls != 2 {
		t.Errorf("executor 桶 = %+v, want {600/60/10/2}（含 run-id keyed 标记行）", members[0])
	}
	if members[1].AgentKey != "planner" || members[1].PromptTok != 80777 || members[1].CompletionTok != 300 || members[1].CachedTok != 79777 || members[1].Calls != 3 {
		t.Errorf("planner 桶 = %+v, want {80777/300/79777/3}（含 remainder 标记行）", members[1])
	}
}

// 无命中与空 runID：nil 切片 + nil error（装配层按无成员用量降级）。
func TestRunMemberUsageStats_Empty(t *testing.T) {
	r := setupRunCacheHitTestRepo(t)
	members, err := r.RunMemberUsageStats(context.Background(), "run-missing")
	if err != nil {
		t.Fatalf("RunMemberUsageStats: %v", err)
	}
	if members != nil {
		t.Errorf("members = %+v, want nil for unknown run", members)
	}
	members, err = r.RunMemberUsageStats(context.Background(), "  ")
	if err != nil {
		t.Fatalf("RunMemberUsageStats blank id: %v", err)
	}
	if members != nil {
		t.Errorf("members = %+v, want nil for blank run id", members)
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
