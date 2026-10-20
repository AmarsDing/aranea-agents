package data

import (
	"context"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// 自治理图谱 M1-2：knowledge_access_log 写入 + base-level 激活分聚合。
// 契约：频次单调（访问多的分高）、新近性单调（同次数 recent > old）、
// 无命中史文档不出现、collection/docIDs 过滤生效。

func setupAccessLogRepo(t *testing.T) bizknowledge.AccessLogRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	repo, ok := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, dialect: DialectPostgres},
		loggateway.NewNoop()).(bizknowledge.AccessLogRepo)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.AccessLogRepo")
	}
	return repo
}

func TestKnowledgeRepo_AccessLog_RoundTrip(t *testing.T) {
	repo := setupAccessLogRepo(t)
	ctx := context.Background()

	entries := []bizknowledge.AccessLogEntry{
		{CollectionID: "c1", DocID: "d1", QueryHash: "q1"},
		{CollectionID: "c1", DocID: "d2", QueryHash: "q1"},
		{CollectionID: "c1", DocID: "d1", QueryHash: "q2"},
	}
	if err := repo.LogAccess(ctx, entries); err != nil {
		t.Fatalf("LogAccess: %v", err)
	}
	if err := repo.LogAccess(ctx, nil); err != nil {
		t.Fatalf("LogAccess empty: %v", err)
	}

	scores, err := repo.BaseLevelScores(ctx, "c1", []string{"d1", "d2", "d3"})
	if err != nil {
		t.Fatalf("BaseLevelScores: %v", err)
	}
	// 频次单调：d1（2 次）> d2（1 次）。
	if scores["d1"] <= scores["d2"] {
		t.Errorf("frequency monotonicity: d1=%v d2=%v", scores["d1"], scores["d2"])
	}
	// 无命中史文档不出现。
	if _, ok := scores["d3"]; ok {
		t.Errorf("d3 with no history must be absent, got %v", scores["d3"])
	}
	// collection 过滤：c2 无数据。
	other, err := repo.BaseLevelScores(ctx, "c2", []string{"d1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("collection filter broken: %v", other)
	}
	// docIDs 过滤：只问 d2。
	only, err := repo.BaseLevelScores(ctx, "c1", []string{"d2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 1 || only["d2"] <= 0 {
		t.Errorf("docIDs filter broken: %v", only)
	}
}

func TestKnowledgeRepo_BaseLevelScores_Recency(t *testing.T) {
	repo := setupAccessLogRepo(t)
	ctx := context.Background()

	// 同次数：recent 1 次 vs stale 1 次（30 天前）→ recent 分高。
	// 直接 SQL 控 accessed_at，绕开 LogAccess 的 NOW() 默认。
	raw := repo.(*knowledgeRepo).data.Postgres()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_access_log (collection_id, doc_id, accessed_at)
		 VALUES ('c1','recent', NOW()), ('c1','stale', NOW() - INTERVAL '30 days')`); err != nil {
		t.Fatal(err)
	}
	scores, err := repo.BaseLevelScores(ctx, "c1", []string{"recent", "stale"})
	if err != nil {
		t.Fatal(err)
	}
	if scores["recent"] <= scores["stale"] {
		t.Errorf("recency monotonicity: recent=%v stale=%v", scores["recent"], scores["stale"])
	}
}

// ── M1-3 Hebbian 共激活边 ─────────────────────────────────────────────────
// 契约：同批 N 文档两两一条边（无向，规范化方向 doc_id<target_doc_id）；
// 重复共激活 weight_f 累加 η；已衰减（valid_to 置位）边被复活；端点文档必须存在。

func TestKnowledgeRepo_StrengthenCoActivations(t *testing.T) {
	repo := setupAccessLogRepo(t)
	ctx := context.Background()
	raw := repo.(*knowledgeRepo).data.Postgres()

	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2", "d3"} {
		if _, err := raw.ExecContext(ctx,
			`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ($1,'c1','s')`, id); err != nil {
			t.Fatal(err)
		}
	}
	coact, ok := repo.(bizknowledge.CoActivationRepo)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.CoActivationRepo")
	}

	// 3 文档同批 → 3 条边（d1-d2, d1-d3, d2-d3）。
	if err := coact.StrengthenCoActivations(ctx, "c1", []string{"d2", "d1", "d3"}, 0.1); err != nil {
		t.Fatalf("strengthen: %v", err)
	}
	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_links WHERE link_type='co_activated'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("co_activated edges = %d, want 3", n)
	}
	// 规范化方向：doc_id < target_doc_id。
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_links WHERE link_type='co_activated' AND doc_id >= target_doc_id`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("edge direction not normalized: %d rows with doc_id >= target_doc_id", n)
	}

	// 二次同批 → weight_f 累加为 2η（±浮点容差）。
	if err := coact.StrengthenCoActivations(ctx, "c1", []string{"d1", "d2"}, 0.1); err != nil {
		t.Fatalf("strengthen 2: %v", err)
	}
	var w float64
	if err := raw.QueryRowContext(ctx,
		`SELECT weight_f FROM knowledge_links WHERE link_type='co_activated' AND doc_id='d1' AND target_doc_id='d2'`).Scan(&w); err != nil {
		t.Fatal(err)
	}
	if w < 0.19 || w > 0.21 {
		t.Errorf("weight_f = %v, want ~0.2 (2η)", w)
	}

	// 衰减置位后再次被共激活 → 复活（valid_to 归 NULL）。
	if _, err := raw.ExecContext(ctx,
		`UPDATE knowledge_links SET valid_to = NOW() WHERE link_type='co_activated' AND doc_id='d1' AND target_doc_id='d3'`); err != nil {
		t.Fatal(err)
	}
	if err := coact.StrengthenCoActivations(ctx, "c1", []string{"d1", "d3"}, 0.1); err != nil {
		t.Fatalf("strengthen 3: %v", err)
	}
	var validTo *time.Time
	if err := raw.QueryRowContext(ctx,
		`SELECT valid_to FROM knowledge_links WHERE link_type='co_activated' AND doc_id='d1' AND target_doc_id='d3'`).Scan(&validTo); err != nil {
		t.Fatal(err)
	}
	if validTo != nil {
		t.Errorf("decayed edge not revived: valid_to = %v", *validTo)
	}

	// 端点缺失（文档已删）→ 跳过该对，不报错不写入。
	if err := coact.StrengthenCoActivations(ctx, "c1", []string{"d1", "ghost"}, 0.1); err != nil {
		t.Fatalf("strengthen with ghost endpoint: %v", err)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_links WHERE link_type='co_activated' AND (doc_id='ghost' OR target_doc_id='ghost')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("ghost endpoint edge written: %d", n)
	}

	// 单文档无对可写（幂等空操作）。
	if err := coact.StrengthenCoActivations(ctx, "c1", []string{"d1"}, 0.1); err != nil {
		t.Fatalf("single doc strengthen: %v", err)
	}
}

// G-15 审计足迹：session/user/agent/rank/score/latency 随命中一起落库并可读回。
// 自建表避开 EnsureKnowledgeSchema 的 vector 扩展依赖（本测试 PG 容器无 pgvector）。
func TestKnowledgeRepo_AccessLog_G15ColumnsRoundTrip(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS knowledge_access_log (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			doc_id TEXT NOT NULL,
			accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			query_hash TEXT,
			session_id TEXT,
			user_id TEXT,
			agent_id TEXT,
			"rank" INTEGER NOT NULL DEFAULT 0,
			score DOUBLE PRECISION NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_access_log_doc_idx
			ON knowledge_access_log (collection_id, doc_id, accessed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS knowledge_access_log_user_idx
			ON knowledge_access_log (user_id, accessed_at DESC)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	repo, ok := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, dialect: DialectPostgres},
		loggateway.NewNoop()).(bizknowledge.AccessLogRepo)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.AccessLogRepo")
	}
	raw := repo.(*knowledgeRepo).data.Postgres()

	entries := []bizknowledge.AccessLogEntry{
		{CollectionID: "c1", DocID: "d1", QueryHash: "q1",
			SessionID: "sess-1", UserID: "u-9", AgentID: "agent-a",
			Rank: 1, Score: 0.95, LatencyMs: 42},
		{CollectionID: "c1", DocID: "d2", QueryHash: "q1",
			SessionID: "sess-1", UserID: "u-9", AgentID: "agent-a",
			Rank: 3, Score: 0.71, LatencyMs: 42},
	}
	if err := repo.LogAccess(ctx, entries); err != nil {
		t.Fatalf("LogAccess: %v", err)
	}

	var sessionID, userID, agentID string
	var rank int
	var score float64
	var latencyMs int
	if err := raw.QueryRowContext(ctx,
		`SELECT session_id, user_id, agent_id, "rank", score, latency_ms
		 FROM knowledge_access_log WHERE collection_id='c1' AND doc_id='d1' ORDER BY id DESC LIMIT 1`).Scan(
		&sessionID, &userID, &agentID, &rank, &score, &latencyMs); err != nil {
		t.Fatalf("select back: %v", err)
	}
	if sessionID != "sess-1" || userID != "u-9" || agentID != "agent-a" {
		t.Errorf("identity = (%q,%q,%q), want (sess-1,u-9,agent-a)", sessionID, userID, agentID)
	}
	if rank != 1 || score != 0.95 || latencyMs != 42 {
		t.Errorf("rank/score/latency = (%d,%v,%d), want (1,0.95,42)", rank, score, latencyMs)
	}
	// 上下文列永不落 NULL（审计查询可按空串过滤，UNNEST 空串 = ''）。
	var nulls int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_access_log
		 WHERE collection_id='c1' AND (session_id IS NULL OR user_id IS NULL OR agent_id IS NULL)`).Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 0 {
		t.Errorf("identity columns must never be NULL, got %d rows", nulls)
	}
}
