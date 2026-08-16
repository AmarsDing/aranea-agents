package data

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 自治理知识图谱 M1 时序地基 ─────────────────────────────────────────────
// 两条路径必须收敛到同一最终形态：
//   fresh 库：EnsureKnowledgeSchema 一步到位建新形态；
//   存量库：迁移 20261220 补列 + 升级唯一索引 + 回填 valid_from。
// 契约：links 双时态（valid_from/valid_to + recorded_at）、谓词 relation、
// 浮点权重 weight_f、置信度 confidence；access_log 承载检索命中。

var knowledgeLinkBitemporalCols = []string{
	"relation", "weight_f", "confidence", "valid_from", "valid_to", "recorded_at",
}

func assertKnowledgeLinksBitemporalShape(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) {
	t.Helper()
	ctx := context.Background()
	for _, col := range knowledgeLinkBitemporalCols {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.columns
			 WHERE table_name='knowledge_links' AND column_name=$1`, col).Scan(&n); err != nil {
			t.Fatalf("check column %s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("knowledge_links.%s missing", col)
		}
	}
	// 唯一索引含 relation（COALESCE 表达式键）。
	var def string
	if err := db.QueryRowContext(ctx,
		`SELECT indexdef FROM pg_indexes WHERE indexname='knowledge_links_unique'`).Scan(&def); err != nil {
		t.Fatalf("knowledge_links_unique indexdef: %v", err)
	}
	if !strings.Contains(def, "relation") {
		t.Errorf("knowledge_links_unique = %q, want relation in key", def)
	}
	// access_log 表存在。
	var reg *string
	if err := db.QueryRowContext(ctx,
		`SELECT to_regclass('knowledge_access_log')::text`).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	if reg == nil || *reg == "" {
		t.Error("knowledge_access_log missing")
	}
}

func TestEnsureKnowledgeSchema_BitemporalFreshShape(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	assertKnowledgeLinksBitemporalShape(t, db)

	// 默认值契约：新边 weight_f/confidence=1.0、valid_from 非空、valid_to 为 NULL（当前有效）。
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ($1,'c1','s')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type)
		 VALUES ('c1','d1','d2','explicit')`); err != nil {
		t.Fatal(err)
	}
	var weightF, confidence float64
	var validFrom time.Time
	var validTo *time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT weight_f, confidence, valid_from, valid_to FROM knowledge_links
		 WHERE doc_id='d1'`).Scan(&weightF, &confidence, &validFrom, &validTo); err != nil {
		t.Fatal(err)
	}
	if weightF != 1.0 || confidence != 1.0 {
		t.Errorf("defaults weight_f=%v confidence=%v, want 1.0/1.0", weightF, confidence)
	}
	if validFrom.IsZero() {
		t.Error("valid_from zero, want DEFAULT NOW()")
	}
	if validTo != nil {
		t.Errorf("valid_to = %v, want NULL (active edge)", *validTo)
	}

	// access_log 可写。
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_access_log (collection_id, doc_id, query_hash) VALUES ('c1','d1','q1')`); err != nil {
		t.Fatalf("access_log insert: %v", err)
	}
}

// 存量库路径：旧形态 links（无新列 + 旧唯一索引）经 20261220 迁移升级。
func TestMigration20261220_UpgradesLegacyLinks(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	legacy := []string{
		`CREATE TABLE knowledge_collections (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL, dim INT NOT NULL DEFAULT 1536,
			status TEXT NOT NULL DEFAULT 'active',
			document_count INT NOT NULL DEFAULT 0, chunk_count INT NOT NULL DEFAULT 0,
			workspace TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE knowledge_documents (
			id TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			source TEXT NOT NULL, mime_type TEXT NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL DEFAULT 0, chunk_count INT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending', error_message TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE knowledge_links (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			target_doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			link_type TEXT NOT NULL, context TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE UNIQUE INDEX knowledge_links_unique ON knowledge_links(doc_id, target_doc_id, link_type)`,
	}
	for _, stmt := range legacy {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("legacy shape: %v", err)
		}
	}
	// 旧数据：created_at 落在过去，验证 valid_from 回填取 created_at。
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ($1,'c1','s')`, id); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-72 * time.Hour)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, created_at)
		 VALUES ('c1','d1','d2','explicit',$1)`, past); err != nil {
		t.Fatal(err)
	}

	run := func() {
		if err := executeSQLFileWithDialect(ctx, db,
			"sql/migrations/20261220_knowledge_links_bitemporal.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	run()
	assertKnowledgeLinksBitemporalShape(t, db)

	// 回填：valid_from 取旧 created_at（±1s 容差）。
	var validFrom time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT valid_from FROM knowledge_links WHERE doc_id='d1'`).Scan(&validFrom); err != nil {
		t.Fatal(err)
	}
	if validFrom.Sub(past) > time.Second || past.Sub(validFrom) > time.Second {
		t.Errorf("valid_from = %v, want backfilled from created_at %v", validFrom, past)
	}
// 幂等重跑。
	run()
}

// M1-4：ListActiveLinks 只回 active 边（valid_to IS NULL），携带 weight_f/relation。
func TestKnowledgeRepo_ListActiveLinks(t *testing.T) {
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
	// active 边 d1-d2（weight_f=2.5, relation='depends-on'）；过期边 d1-d3。
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, weight_f)
		 VALUES ('c1','d1','d2','semantic','depends-on',2.5),
		        ('c1','d1','d3','explicit','',1.0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx,
		`UPDATE knowledge_links SET valid_to = NOW() WHERE doc_id='d1' AND target_doc_id='d3'`); err != nil {
		t.Fatal(err)
	}

	reader, ok := repo.(bizknowledge.ActiveLinkReader)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.ActiveLinkReader")
	}
	links, err := reader.ListActiveLinks(ctx, "c1", []string{"d1"})
	if err != nil {
		t.Fatalf("ListActiveLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("active links = %d, want 1 (expired edge must be excluded): %+v", len(links), links)
	}
	l := links[0]
	if l.TargetDocID != "d2" || l.LinkType != "semantic" || l.Relation != "depends-on" || l.WeightF != 2.5 {
		t.Errorf("link projection wrong: %+v", l)
	}
	// 反向触及同样命中（d2 作为查询端点）。
	links, err = reader.ListActiveLinks(ctx, "c1", []string{"d2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].DocID != "d1" {
		t.Errorf("reverse touch broken: %+v", links)
	}
}
