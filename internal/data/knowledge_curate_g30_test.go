package data

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// G-30 治理中心：提案列表 offset 分页——items 走 LIMIT/OFFSET，total 同 WHERE 口径。
// 自建表避开 EnsureKnowledgeSchema 的 vector 扩展依赖（本测试 PG 容器无 pgvector）。
func TestKnowledgeRepo_ListGovernanceProposals_Pagination(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS knowledge_governance_proposal (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			risk TEXT NOT NULL DEFAULT 'low',
			status TEXT NOT NULL DEFAULT 'pending',
			payload JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ
		)`); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	// 同库两状态 + 异库一行（id 越小越早插入）。
	for _, row := range []struct {
		col, kind, risk, status string
	}{
		{"c1", "stale", "low", "pending"},
		{"c1", "conflict", "high", "applied"},
		{"c1", "orphan", "low", "pending"},
		{"c1", "moc_emerge", "low", "pending"},
		{"c2", "conflict", "high", "pending"},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO knowledge_governance_proposal (collection_id, kind, risk, status, payload)
			 VALUES ($1,$2,$3,$4,'{}')`, row.col, row.kind, row.risk, row.status); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	repo, ok := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, dialect: DialectPostgres},
		loggateway.NewNoop()).(bizknowledge.KnowledgeCurateRepo)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.KnowledgeCurateRepo")
	}

	// c1 全部：total=4；第一页 limit=2（id DESC → 最新两行）。
	views, total, err := repo.ListGovernanceProposals(ctx, "c1", "", 2, 0)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if total != 4 || len(views) != 2 {
		t.Fatalf("page1 = (total %d, items %d), want (4, 2)", total, len(views))
	}
	if views[0].ID <= views[1].ID {
		t.Errorf("id DESC ordering broken: %v", []int64{views[0].ID, views[1].ID})
	}
	// 第二页：offset=2 → 旧两行。
	views2, total2, err := repo.ListGovernanceProposals(ctx, "c1", "", 2, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if total2 != 4 || len(views2) != 2 {
		t.Fatalf("page2 = (total %d, items %d), want (4, 2)", total2, len(views2))
	}
	if views2[0].ID != views[1].ID-1 {
		t.Errorf("offset continuity: page2 first id = %d, want %d", views2[0].ID, views[1].ID-1)
	}
	// status 过滤：pending total=3；offset=1 取中间一行。
	views3, total3, err := repo.ListGovernanceProposals(ctx, "c1", "pending", 1, 1)
	if err != nil {
		t.Fatalf("filtered: %v", err)
	}
	if total3 != 3 || len(views3) != 1 {
		t.Fatalf("filtered = (total %d, items %d), want (3, 1)", total3, len(views3))
	}
	if views3[0].Status != "pending" {
		t.Fatalf("filtered status = %q, want pending", views3[0].Status)
	}
	if !views3[0].ResolvedAt.IsZero() {
		t.Errorf("pending row must have zero resolved_at: %v", views3[0].ResolvedAt)
	}
	// offset 越界：items 空但 total 仍为总量（分页 UI 据此清零游标）。
	views4, total4, err := repo.ListGovernanceProposals(ctx, "c1", "", 50, 100)
	if err != nil {
		t.Fatalf("overflow: %v", err)
	}
	if total4 != 4 || len(views4) != 0 {
		t.Fatalf("overflow = (total %d, items %d), want (4, 0)", total4, len(views4))
	}
}
