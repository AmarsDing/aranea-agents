package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// D7-F5 orchestration_cache_entries 分行持久化 repo 测试（PG-only；SQLite
// 支持已于 2026-10-21 全面移除）。同一 SQL 经 Dialect 占位符重写跑 PG 一侧。
// ---------------------------------------------------------------------------

func newOrchestrationCacheRepoTestDB(t *testing.T) (*Data, *orchestrationCacheRepo) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS orchestration_cache_entries (
  workspace_id TEXT NOT NULL DEFAULT '',
  task_pattern TEXT NOT NULL,
  entry_json TEXT NOT NULL,
  dq_score REAL NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, task_pattern)
)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	repo, ok := NewOrchestrationCacheRepo(d).(*orchestrationCacheRepo)
	if !ok {
		t.Fatal("type assertion failed")
	}
	return d, repo
}

func TestOrchestrationCacheRepo_UpsertLoadDelete_RoundTrip(t *testing.T) {
	_, repo := newOrchestrationCacheRepoTestDB(t)
	ctx := context.Background()

	entry := biz.OrchestrationCacheEntry{
		TaskPattern:           "domain:创作/文学",
		WorkspaceID:           "ws-a",
		DomainPath:            "创作/文学",
		Topology:              biz.TopologyParallel,
		DQScore:               0.85,
		TeamCount:             2,
		AgentKeys:             []string{"agent-a", "agent-b"},
		Specialties:           []string{"创作/文学"},
		PlaybookID:            "pb-lit",
		ConstraintFingerprint: "fp-lit",
		UpdatedAt:             "2099-01-01T00:00:00Z",
	}
	if err := repo.UpsertEntry(ctx, entry); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 更新同键：DQ 衰减，行数不变。
	entry.DQScore = 0.68
	if err := repo.UpsertEntry(ctx, entry); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	// 另一租户同 pattern：独立行。
	other := entry
	other.WorkspaceID = "ws-b"
	other.DQScore = 0.9
	if err := repo.UpsertEntry(ctx, other); err != nil {
		t.Fatalf("upsert tenant-b: %v", err)
	}

	entries, err := repo.LoadEntries(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("rows=%d want 2（同 pattern 跨租户分行）", len(entries))
	}
	byWS := map[string]biz.OrchestrationCacheEntry{}
	for _, e := range entries {
		byWS[e.WorkspaceID] = e
	}
	got := byWS["ws-a"]
	if got.DQScore != 0.68 || got.Topology != biz.TopologyParallel || len(got.AgentKeys) != 2 {
		t.Fatalf("ws-a round-trip lost fields: %+v", got)
	}
	if got.PlaybookID != "pb-lit" || got.ConstraintFingerprint != "fp-lit" {
		t.Fatalf("provenance lost in persistence: %+v", got)
	}
	if byWS["ws-b"].DQScore != 0.9 {
		t.Fatalf("ws-b row must not be touched by ws-a updates: %+v", byWS["ws-b"])
	}

	if err := repo.DeleteEntry(ctx, "ws-a", "domain:创作/文学"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, err = repo.LoadEntries(ctx)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if len(entries) != 1 || entries[0].WorkspaceID != "ws-b" {
		t.Fatalf("delete must be workspace-scoped, got %+v", entries)
	}
	// 删除不存在行是幂等 no-op。
	if err := repo.DeleteEntry(ctx, "ws-a", "domain:创作/文学"); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestOrchestrationCacheRepo_WorkspaceNormalization(t *testing.T) {
	_, repo := newOrchestrationCacheRepoTestDB(t)
	ctx := context.Background()
	// 空 workspace 归一 default（legacy 防御）。
	entry := biz.OrchestrationCacheEntry{
		TaskPattern: "pat-1", Topology: biz.TopologyDirect, DQScore: 0.9,
		UpdatedAt: "2099-01-01T00:00:00Z",
	}
	if err := repo.UpsertEntry(ctx, entry); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	entries, err := repo.LoadEntries(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(entries) != 1 || entries[0].WorkspaceID != "default" {
		t.Fatalf("empty workspace must normalize to default bucket, got %+v", entries)
	}
}
