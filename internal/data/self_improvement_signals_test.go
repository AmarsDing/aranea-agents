package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── DB-backed signal adapters ────────────────────────────────────────────────

// setupSignalRepo creates the raw mirror of model_token_usage_events (raw-DDL
// table, not in Ent auto-migration) plus the Ent-managed eval_runs table.
func setupSignalRepo(t *testing.T) (*SelfImprovementSignalRepo, *ent.Client, context.Context) {
	t.Helper()
	client, rawDB := testhelper.SetupTestPG(t)
	_, err := rawDB.ExecContext(context.Background(), `CREATE TABLE model_token_usage_events (
		id TEXT PRIMARY KEY,
		occurred_at TEXT NOT NULL,
		agent_key TEXT NOT NULL DEFAULT '',
		usage_kind TEXT NOT NULL DEFAULT 'chat',
		total_tokens INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'success',
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatal(err)
	}
	// 需要 raw SQL（RWDB）访问 model_token_usage_events，故用 SetEntClientForTest
	// 而非 newDataFromClient（后者不初始化 rawDB/rwDB）。
	d := &Data{}
	d.SetEntClientForTest(client, rawDB, loggateway.NewNoop())
	return NewSelfImprovementSignalRepo(d), client, context.Background()
}

func insertUsageEvent(t *testing.T, repo *SelfImprovementSignalRepo, ctx context.Context, id string, at time.Time, agentKey string, totalTokens, latencyMs int64, status, errCode, errMsg string) {
	t.Helper()
	_, err := repo.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO model_token_usage_events (id, occurred_at, agent_key, total_tokens, latency_ms, status, error_code, error_message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, at.UTC().Format(time.RFC3339Nano), agentKey, totalTokens, latencyMs, status, errCode, errMsg)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignalRepo_ListErrorClusters(t *testing.T) {
	repo, _, ctx := setupSignalRepo(t)
	now := time.Now().UTC()

	// nil_pointer ×6（达阈值 5），rate_limit ×4（未达），success 事件不计。
	for i := 0; i < 6; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("e-nil-%d", i), now.Add(-time.Duration(i)*time.Hour), "agent-a", 0, 100, "failed", "nil_pointer_dereference", fmt.Sprintf("nil deref #%d", i))
	}
	for i := 0; i < 4; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("e-rl-%d", i), now.Add(-time.Duration(i)*time.Hour), "agent-b", 0, 100, "failed", "rate_limit", "429")
	}
	// 窗口外的 nil_pointer（8 天前）不计。
	insertUsageEvent(t, repo, ctx, "e-old", now.Add(-8*24*time.Hour), "agent-a", 0, 100, "failed", "nil_pointer_dereference", "old")
	insertUsageEvent(t, repo, ctx, "e-ok", now, "agent-a", 100, 100, "success", "", "")

	clusters, err := repo.ListErrorClusters(ctx, now.Add(-7*24*time.Hour), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("len = %d, want 1 (rate_limit 未达阈值)", len(clusters))
	}
	c := clusters[0]
	if c.ErrorCode != "nil_pointer_dereference" {
		t.Errorf("ErrorCode = %q", c.ErrorCode)
	}
	if c.Count != 6 {
		t.Errorf("Count = %d, want 6（窗口外 1 条不计）", c.Count)
	}
	// 采样取最新一条的 message 与 agent。
	if c.SampleMessage != "nil deref #0" {
		t.Errorf("SampleMessage = %q, want latest", c.SampleMessage)
	}
	if c.Component != "agent-a" {
		t.Errorf("Component = %q", c.Component)
	}
	if c.LastSeen.IsZero() {
		t.Error("LastSeen zero")
	}
}

func TestSignalRepo_Snapshot(t *testing.T) {
	repo, _, ctx := setupSignalRepo(t)
	now := time.Now().UTC()

	// 窗口内：3 success + 1 failed → error_rate=0.25；latency 100/200/300/400 → p95≈400。
	insertUsageEvent(t, repo, ctx, "s-1", now.Add(-10*time.Minute), "agent-a", 100, 100, "success", "", "")
	insertUsageEvent(t, repo, ctx, "s-2", now.Add(-20*time.Minute), "agent-a", 100, 200, "success", "", "")
	insertUsageEvent(t, repo, ctx, "s-3", now.Add(-30*time.Minute), "agent-a", 100, 300, "success", "", "")
	insertUsageEvent(t, repo, ctx, "s-4", now.Add(-40*time.Minute), "agent-a", 100, 400, "failed", "boom", "x")
	// 窗口外（2h 前）：不计。
	insertUsageEvent(t, repo, ctx, "s-old", now.Add(-2*time.Hour), "agent-a", 100, 9999, "failed", "old", "x")

	snap, err := repo.Snapshot(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ErrorRate < 0.24 || snap.ErrorRate > 0.26 {
		t.Errorf("ErrorRate = %v, want ≈0.25", snap.ErrorRate)
	}
	if snap.P95MS < 350 || snap.P95MS > 400 {
		t.Errorf("P95MS = %v, want ≈400", snap.P95MS)
	}
	if snap.AlertCount != 0 {
		t.Errorf("AlertCount = %d, want 0（P4 偏差：无 fired-alert 表）", snap.AlertCount)
	}

	// 空窗口 → 全零快照而非错误。
	empty, err := repo.Snapshot(ctx, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if empty.ErrorRate != 0 || empty.P95MS != 0 {
		t.Errorf("空窗口快照 = %+v, want zeros", empty)
	}
}

func TestSignalRepo_GetStepLatencyStats(t *testing.T) {
	repo, _, ctx := setupSignalRepo(t)
	now := time.Now().UTC()

	// baseline 窗口 [7d, 24h)：agent-a latency=100；current [24h, now)：latency=500。
	for i := 0; i < 20; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("b-%d", i), now.Add(-time.Duration(25+i)*time.Hour), "agent-a", 100, 100, "success", "", "")
	}
	for i := 0; i < 20; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("c-%d", i), now.Add(-time.Duration(i)*time.Hour), "agent-a", 100, 500, "success", "", "")
	}
	// agent-b：仅 current 有数据（无基线）→ baseline=0，触发器会跳过。
	insertUsageEvent(t, repo, ctx, "c-b0", now.Add(-time.Hour), "agent-b", 100, 999, "success", "", "")

	stats, err := repo.GetStepLatencyStats(ctx, "7d", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("len = %d, want 2 agents", len(stats))
	}
	byStep := map[string]biz.StepLatencyStat{}
	for _, s := range stats {
		byStep[s.StepID] = s
	}
	a := byStep["agent-a"]
	if a.BaselineP95MS < 90 || a.BaselineP95MS > 110 {
		t.Errorf("agent-a baseline p95 = %v, want ≈100", a.BaselineP95MS)
	}
	if a.CurrentP95MS < 450 || a.CurrentP95MS > 550 {
		t.Errorf("agent-a current p95 = %v, want ≈500", a.CurrentP95MS)
	}
	if a.SampleCount != 20 {
		t.Errorf("agent-a samples = %d, want 20（current 窗口）", a.SampleCount)
	}
	if b := byStep["agent-b"]; b.BaselineP95MS != 0 {
		t.Errorf("agent-b baseline = %v, want 0（无基线数据）", b.BaselineP95MS)
	}
}

func TestSignalRepo_GetTokenUsageStats(t *testing.T) {
	repo, _, ctx := setupSignalRepo(t)
	now := time.Now().UTC()

	for i := 0; i < 10; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("tb-%d", i), now.Add(-time.Duration(30+i)*time.Hour), "agent-a", 1000, 10, "success", "", "")
	}
	for i := 0; i < 10; i++ {
		insertUsageEvent(t, repo, ctx, fmt.Sprintf("tc-%d", i), now.Add(-time.Duration(i)*time.Hour), "agent-a", 2000, 10, "success", "", "")
	}

	stats, err := repo.GetTokenUsageStats(ctx, "7d", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("len = %d, want 1", len(stats))
	}
	s := stats[0]
	if s.Scope != "agent-a" {
		t.Errorf("Scope = %q", s.Scope)
	}
	if s.BaselineTokens != 1000 {
		t.Errorf("BaselineTokens = %v, want 1000", s.BaselineTokens)
	}
	if s.CurrentTokens != 2000 {
		t.Errorf("CurrentTokens = %v, want 2000", s.CurrentTokens)
	}
}

func TestSignalRepo_EvalBaselines(t *testing.T) {
	repo, client, ctx := setupSignalRepo(t)
	now := time.Now().UTC()

	// dataset 需先建（eval_runs.dataset_id FK 到 eval_datasets）。
	if _, err := client.EvalDataset.Create().
		SetID("ds-1").SetWorkspace("").SetName("d1").
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	mkRun := func(id, ds, agent string, score float64, at time.Time, status string) {
		t.Helper()
		if _, err := client.EvalRun.Create().
			SetID(id).SetDatasetID(ds).SetAgentID(agent).
			SetStatus(status).
			SetExactMatchScore(score).SetContainsMatchScore(score).
			SetLlmJudgeScore(score).SetToolCallAccuracy(score).
			SetCreatedAt(at.Format(time.RFC3339Nano)).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// r1（旧）→ r2（新，同 ds/agent 退化）；r3 更新但属别的 agent —— previous 必须按同 ds+agent 配对。
	mkRun("r1", "ds-1", "agent-a", 0.80, now.Add(-48*time.Hour), "completed")
	mkRun("r2", "ds-1", "agent-a", 0.60, now.Add(-24*time.Hour), "completed")
	mkRun("r3", "ds-1", "agent-b", 0.95, now.Add(-time.Hour), "completed")
	mkRun("r4", "ds-1", "agent-a", 0.99, now, "running") // 非 completed 不参与

	latest, err := repo.GetLatestBaseline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.RunID != "r3" {
		t.Fatalf("latest = %+v, want r3（全局最新 completed）", latest)
	}
	if latest.Score < 0.94 || latest.Score > 0.96 {
		t.Errorf("latest.Score = %v, want 0.95", latest.Score)
	}

	// previous 必须是 r3 同 ds+agent 的前一条：agent-b 只有 r3 一条 → nil。
	prev, err := repo.GetPreviousBaseline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if prev != nil {
		t.Errorf("previous = %+v, want nil（agent-b 仅此一条 completed）", prev)
	}

	// latest 为 r2 的场景（删 r3）：previous 应配对 r1。
	if err := client.EvalRun.DeleteOneID("r3").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	latest, err = repo.GetLatestBaseline(ctx)
	if err != nil || latest == nil || latest.RunID != "r2" {
		t.Fatalf("latest = %+v, want r2", latest)
	}
	prev, err = repo.GetPreviousBaseline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if prev == nil || prev.RunID != "r1" {
		t.Fatalf("previous = %+v, want r1（同 ds+agent 配对）", prev)
	}
	if prev.Score < 0.79 || prev.Score > 0.81 {
		t.Errorf("prev.Score = %v, want 0.80", prev.Score)
	}
}

// ── File-based test-run reader ───────────────────────────────────────────────

func writeRoundFile(t *testing.T, dir, name string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTestRunFileReader_ConsecutiveFailures(t *testing.T) {
	dir := t.TempDir()
	// 3 轮（新→旧）：round3 最新。TestFoo 3 轮连败；TestBar 仅最新 1 轮；TestBaz 最新一轮通过（中断）。
	writeRoundFile(t, dir, "round1.json", `{"round":"2026-07-27T10:00:00Z","failures":[
		{"package":"./internal/biz","test_name":"TestFoo","error":"assert 1"},
		{"package":"./internal/data","test_name":"TestBaz","error":"assert 2"}]}`)
	writeRoundFile(t, dir, "round2.json", `{"round":"2026-07-28T10:00:00Z","failures":[
		{"package":"./internal/biz","test_name":"TestFoo","error":"assert 3"},
		{"package":"./internal/data","test_name":"TestBaz","error":"assert 4"}]}`)
	writeRoundFile(t, dir, "round3.json", `{"round":"2026-07-29T10:00:00Z","failures":[
		{"package":"./internal/biz","test_name":"TestFoo","error":"assert 5"},
		{"package":"./internal/svc","test_name":"TestBar","error":"assert 6"}]}`)

	r := NewTestRunFileReader(dir)
	failures, err := r.ListRecentFailures(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("len = %d, want 1（TestFoo 3 连；TestBar 仅 1 轮；TestBaz 最新轮通过）", len(failures))
	}
	f := failures[0]
	if f.TestName != "TestFoo" || f.ConsecutiveRounds != 3 {
		t.Errorf("failure = %+v", f)
	}
	if f.LastError != "assert 5" {
		t.Errorf("LastError = %q, want 最新轮的断言", f.LastError)
	}
	if f.LastSeen.Format("2006-01-02") != "2026-07-29" {
		t.Errorf("LastSeen = %v", f.LastSeen)
	}
}

func TestTestRunFileReader_MissingDirReturnsEmpty(t *testing.T) {
	r := NewTestRunFileReader(filepath.Join(t.TempDir(), "nonexistent"))
	failures, err := r.ListRecentFailures(context.Background(), 2)
	if err != nil || len(failures) != 0 {
		t.Errorf("missing dir: failures=%v err=%v, want empty/nil", failures, err)
	}
}

func TestTestRunFileReader_SkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	writeRoundFile(t, dir, "bad.json", `not json`)
	writeRoundFile(t, dir, "round1.json", `{"round":"2026-07-29T10:00:00Z","failures":[{"package":"./x","test_name":"TestA","error":"e"}]}`)
	r := NewTestRunFileReader(dir)
	failures, err := r.ListRecentFailures(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].TestName != "TestA" {
		t.Errorf("failures = %+v", failures)
	}
}
