package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── P3 M3：Agent Case 召回（word_similarity 中文短查询 + 空查询降级）─────────

func setupAgentCaseRepo(t *testing.T) *memoryAgentCaseRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	// pg_trgm 扩展装在 public schema（库级唯一）；测试 schema 的 search_path 不含
	// public，`%>`/word_similarity 解析失败会静默走降级路径。钉单连接后把 public
	// 追加进 search_path（同 knowledge_search_path_test.go 的 pgvector 处理）。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm`); err != nil {
		t.Fatalf("create pg_trgm: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS memory_agent_cases (
		  id TEXT PRIMARY KEY,
		  agent_id TEXT NOT NULL,
		  user_id TEXT NOT NULL DEFAULT '',
		  source_session_id TEXT NOT NULL,
		  goal TEXT NOT NULL DEFAULT '',
		  approach TEXT NOT NULL DEFAULT '',
		  outcome TEXT NOT NULL DEFAULT 'partial',
		  outcome_summary TEXT NOT NULL DEFAULT '',
		  pitfalls TEXT NOT NULL DEFAULT '',
		  tools_used TEXT NOT NULL DEFAULT '[]',
		  quality REAL NOT NULL DEFAULT 0.5,
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_agent_cases_agent_session ON memory_agent_cases(agent_id, source_session_id)`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	return &memoryAgentCaseRepo{data: d}
}

func seedAgentCase(t *testing.T, repo *memoryAgentCaseRepo, c biz.AgentCase) {
	t.Helper()
	if err := repo.UpsertAgentCase(context.Background(), c); err != nil {
		t.Fatalf("seed case %s: %v", c.SourceSessionID, err)
	}
}

func TestMemoryAgentCaseRepo_RecallAgentCases_ChineseShortQuery(t *testing.T) {
	repo := setupAgentCaseRepo(t)
	ctx := context.Background()
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-1", SourceSessionID: "s1",
		Goal: "批量导入用户数据", Approach: "先小批量试跑再分批提交",
		Outcome: biz.AgentCaseOutcomeSuccess, Quality: 0.9,
	})
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-1", SourceSessionID: "s2",
		Goal: "修复线上事故", Pitfalls: "直接全量刷新缓存导致雪崩",
		Outcome: biz.AgentCaseOutcomeFailure, Quality: 0.8,
	})
	// 其他 agent 的 case 不得串扰。
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-2", SourceSessionID: "s3",
		Goal: "批量导入用户数据", Outcome: biz.AgentCaseOutcomeSuccess, Quality: 0.9,
	})

	// 中文短查询命中 goal（连续子串——word_similarity 匹配文本连续区间，
	// 非连续 token 组合如"导入数据"相似度仅 0.2 不命中，psql 实测）。
	got, err := repo.RecallAgentCases(ctx, "ag-1", "批量导入", 3)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got) != 1 || got[0].SourceSessionID != "s1" {
		t.Fatalf("goal hit: want [s1], got %+v", got)
	}
	if got[0].Approach != "先小批量试跑再分批提交" || got[0].Outcome != biz.AgentCaseOutcomeSuccess {
		t.Fatalf("fields must round-trip, got %+v", got[0])
	}
	// 中文短查询命中 pitfalls。
	got, err = repo.RecallAgentCases(ctx, "ag-1", "缓存导致雪崩", 3)
	if err != nil {
		t.Fatalf("recall pitfalls: %v", err)
	}
	if len(got) != 1 || got[0].SourceSessionID != "s2" {
		t.Fatalf("pitfalls hit: want [s2], got %+v", got)
	}
	// 不存在的词不得误中。
	got, err = repo.RecallAgentCases(ctx, "ag-1", "量子引力波xx", 3)
	if err != nil {
		t.Fatalf("recall miss: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("miss query must return empty, got %+v", got)
	}
}

func TestMemoryAgentCaseRepo_RecallAgentCases_EmptyQueryFallsBackRecent(t *testing.T) {
	repo := setupAgentCaseRepo(t)
	ctx := context.Background()
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-1", SourceSessionID: "s1",
		Goal: "低质量任务", Outcome: biz.AgentCaseOutcomePartial, Quality: 0.3,
	})
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-1", SourceSessionID: "s2",
		Goal: "高质量任务", Outcome: biz.AgentCaseOutcomeSuccess, Quality: 0.95,
	})
	seedAgentCase(t, repo, biz.AgentCase{
		AgentID: "ag-1", SourceSessionID: "s3",
		Goal: "中质量任务", Outcome: biz.AgentCaseOutcomeSuccess, Quality: 0.7,
	})

	// 空 query → 最近高质量排序（quality 优先），limit 生效。
	got, err := repo.RecallAgentCases(ctx, "ag-1", "", 2)
	if err != nil {
		t.Fatalf("recall recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit=2 must return 2, got %d", len(got))
	}
	if got[0].SourceSessionID != "s2" || got[1].SourceSessionID != "s3" {
		t.Fatalf("quality-desc order: want [s2 s3], got [%s %s]", got[0].SourceSessionID, got[1].SourceSessionID)
	}
}

func TestMemoryAgentCaseRepo_RecallAgentCases_NilSafe(t *testing.T) {
	var repo *memoryAgentCaseRepo
	got, err := repo.RecallAgentCases(context.Background(), "ag-1", "q", 3)
	if err != nil || got != nil {
		t.Fatalf("nil repo must return (nil, nil), got %v %v", got, err)
	}
	got, err = repo.RecallAgentCases(context.Background(), "", "q", 3)
	if err != nil || got != nil {
		t.Fatalf("empty agentID must return (nil, nil), got %v %v", got, err)
	}
}
