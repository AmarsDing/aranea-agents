package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── P3 M5-B：进化多样性聚合观测（GetDiversityOverview）─────────────────────

func setupUnifiedEvoRepo(t *testing.T) *UnifiedEvolutionRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS unified_evolution_suggestions (
		  id TEXT PRIMARY KEY,
		  target_type TEXT NOT NULL,
		  target_id TEXT NOT NULL,
		  workspace_id TEXT NOT NULL DEFAULT '',
		  action_type TEXT NOT NULL,
		  trigger_source TEXT NOT NULL DEFAULT '',
		  trigger_reason TEXT NOT NULL DEFAULT '',
		  status TEXT NOT NULL DEFAULT 'pending',
		  priority INTEGER NOT NULL DEFAULT 0,
		  draft_body TEXT NOT NULL DEFAULT '',
		  draft_name TEXT NOT NULL DEFAULT '',
		  merge_target_id TEXT NOT NULL DEFAULT '',
		  lifecycle_status TEXT NOT NULL DEFAULT 'draft',
		  sandbox_passed INTEGER NOT NULL DEFAULT 0,
		  sandbox_result TEXT,
		  metadata TEXT,
		  created_at TEXT NOT NULL,
		  approved_by TEXT NOT NULL DEFAULT '',
		  applied_at TEXT
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	return NewUnifiedEvolutionRepo(d, loggateway.NewNoop())
}

func seedUnifiedSuggestion(t *testing.T, repo *UnifiedEvolutionRepo, id, source string, createdAt time.Time, metadata json.RawMessage) {
	t.Helper()
	err := repo.Create(context.Background(), biz.UnifiedEvolutionSuggestion{
		ID: id, TargetType: biz.EvolutionTargetAgent, TargetID: "ag-" + id,
		ActionType: biz.EvolutionActionCreate, TriggerSource: source,
		Status: "pending", LifecycleStatus: "draft",
		Metadata:  metadata,
		CreatedAt: createdAt.UTC(),
	})
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func dimsMetadata(t *testing.T, tools ...string) json.RawMessage {
	t.Helper()
	buf, err := json.Marshal(map[string]any{"dims": map[string]any{"tools": tools}})
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

// 多分桶：按 trigger_source 聚合 count + latest_at，按 count 降序。
func TestUnifiedEvolutionRepo_GetDiversityOverview_Buckets(t *testing.T) {
	repo := setupUnifiedEvoRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)

	seedUnifiedSuggestion(t, repo, "a1", "pattern", now.Add(-1*time.Hour), dimsMetadata(t, "shell_exec"))
	seedUnifiedSuggestion(t, repo, "a2", "pattern", now.Add(-2*time.Hour), nil)
	seedUnifiedSuggestion(t, repo, "a3", biz.CaseDistillTriggerSource, now.Add(-3*time.Hour), dimsMetadata(t, "query_db"))

	got, err := repo.GetDiversityOverview(ctx, since, 5)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 buckets, got %+v", got)
	}
	if got[0].TriggerSource != "pattern" || got[0].Count != 2 {
		t.Fatalf("bucket0=%+v", got[0])
	}
	// LatestAt 取窗口内 MAX(created_at)。
	if got[0].LatestAt.Sub(now.Add(-1*time.Hour)) > time.Second {
		t.Fatalf("latest_at=%v", got[0].LatestAt)
	}
	if got[1].TriggerSource != biz.CaseDistillTriggerSource || got[1].Count != 1 {
		t.Fatalf("bucket1=%+v", got[1])
	}
}

// dims.tools 频次聚合成 TopN；metadata NULL / 无 dims 的行被容忍不影响统计。
func TestUnifiedEvolutionRepo_GetDiversityOverview_TopTools(t *testing.T) {
	repo := setupUnifiedEvoRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)

	seedUnifiedSuggestion(t, repo, "b1", "pattern", now, dimsMetadata(t, "shell_exec", "query_db"))
	seedUnifiedSuggestion(t, repo, "b2", "pattern", now, dimsMetadata(t, "shell_exec"))
	seedUnifiedSuggestion(t, repo, "b3", "pattern", now, dimsMetadata(t, "api_call"))
	seedUnifiedSuggestion(t, repo, "b4", "pattern", now, nil)                        // NULL metadata
	seedUnifiedSuggestion(t, repo, "b5", "pattern", now, json.RawMessage(`{"k":1}`)) // 无 dims 键

	got, err := repo.GetDiversityOverview(ctx, since, 2)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bucket, got %+v", got)
	}
	// shell_exec×2, query_db×1, api_call×1 → top2 = [shell_exec, ...]；
	// 同频次按工具名字典序保证稳定。
	if len(got[0].TopTools) != 2 || got[0].TopTools[0] != "shell_exec" {
		t.Fatalf("top_tools=%v", got[0].TopTools)
	}
}

// since 窗口外的旧行不计入。
func TestUnifiedEvolutionRepo_GetDiversityOverview_SinceFilter(t *testing.T) {
	repo := setupUnifiedEvoRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedUnifiedSuggestion(t, repo, "c1", "pattern", now.Add(-48*time.Hour), dimsMetadata(t, "old_tool"))
	seedUnifiedSuggestion(t, repo, "c2", "pattern", now.Add(-1*time.Hour), dimsMetadata(t, "new_tool"))

	got, err := repo.GetDiversityOverview(ctx, now.Add(-24*time.Hour), 5)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("want 1 row in window, got %+v", got)
	}
	if len(got[0].TopTools) != 1 || got[0].TopTools[0] != "new_tool" {
		t.Fatalf("top_tools=%v", got[0].TopTools)
	}
}

// 空表 / 空窗口 → 空结果而非错误。
func TestUnifiedEvolutionRepo_GetDiversityOverview_Empty(t *testing.T) {
	repo := setupUnifiedEvoRepo(t)
	got, err := repo.GetDiversityOverview(context.Background(), time.Now().UTC(), 5)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty must be empty, got %v %v", got, err)
	}
}
