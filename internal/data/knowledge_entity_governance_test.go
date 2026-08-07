package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── G5-F（B9/B12）：knowledge_entities name_norm 回填 + 冲突组合并迁移 ────────

// setupLegacyEntityDB 建迁移前形态（无 name_norm、无 aliases 表、
// UNIQUE(collection_id, name)），镜像 20261129 迁移针对的存量库。
func setupLegacyEntityDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE knowledge_collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			dim INT NOT NULL DEFAULT 3,
			status TEXT NOT NULL DEFAULT 'active',
			document_count INT NOT NULL DEFAULT 0,
			chunk_count INT NOT NULL DEFAULT 0,
			workspace TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE knowledge_documents (
			id TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			source TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL DEFAULT 0,
			chunk_count INT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'indexed',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE knowledge_entities (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			entity_type TEXT NOT NULL DEFAULT '',
			UNIQUE (collection_id, name)
		)`,
		`CREATE TABLE knowledge_doc_entities (
			collection_id TEXT NOT NULL,
			doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			entity_id BIGINT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
			mentions INT NOT NULL DEFAULT 1,
			PRIMARY KEY (doc_id, entity_id)
		)`,
		`CREATE TABLE knowledge_links (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			target_doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			link_type TEXT NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO knowledge_collections (id, name) VALUES ('c1','vault')`,
		`INSERT INTO knowledge_documents (id, collection_id) VALUES ('d1','c1'),('d2','c1'),('d3','c1')`,
		// 归一化冲突组："AI"/"ai"/"ＡＩ" → 同一 norm；RAG 为单例。
		`INSERT INTO knowledge_entities (collection_id, name, entity_type) VALUES
			('c1','AI','tech'),('c1','ai','tech'),('c1','ＡＩ','concept'),('c1','RAG','tech')`,
		`INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions) VALUES
			('c1','d1',1,2),('c1','d1',2,1),('c1','d2',2,3),('c1','d3',3,1),('c1','d2',4,5)`,
		`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, context) VALUES
			('c1','d1','d2','entity','ai,RAG'),
			('c1','d3','d1','entity','ＡＩ'),
			('c1','d1','d3','explicit','[[c]]')`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed legacy schema: %v\n---\n%s", err, s)
		}
	}
	return db
}

func TestDDLKnowledgeEntityGovernance_LegacyBackfillMerge(t *testing.T) {
	db := setupLegacyEntityDB(t)
	ctx := context.Background()

	if err := ddlKnowledgeEntityGovernance(ctx, db, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("migration: %v", err)
	}

	// 冲突组合并：keeper = id 最小者（1,"AI"），展示名保留首见写法。
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities WHERE collection_id='c1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("entities after merge = %d, want 2 (AI + RAG)", count)
	}
	var name, norm, entityType string
	if err := db.QueryRowContext(ctx,
		`SELECT name, name_norm, entity_type FROM knowledge_entities WHERE id=1`).Scan(&name, &norm, &entityType); err != nil {
		t.Fatal(err)
	}
	if name != "AI" || norm != "ai" || entityType != "tech" {
		t.Errorf("keeper row = (%q,%q,%q), want (AI,ai,tech)", name, norm, entityType)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT name_norm FROM knowledge_entities WHERE name='RAG'`).Scan(&norm); err != nil {
		t.Fatal(err)
	}
	if norm != "rag" {
		t.Errorf("RAG name_norm = %q, want rag", norm)
	}

	// 提及重写：(d1,keeper) mentions = 2+1 冲突合并；其余平移。
	type deKey struct {
		docID    string
		entityID int64
	}
	rows := map[deKey]int{}
	qr, err := db.QueryContext(ctx,
		`SELECT doc_id, entity_id, mentions FROM knowledge_doc_entities ORDER BY doc_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer qr.Close()
	for qr.Next() {
		var docID string
		var entityID int64
		var mentions int
		if err := qr.Scan(&docID, &entityID, &mentions); err != nil {
			t.Fatal(err)
		}
		rows[deKey{docID, entityID}] = mentions
	}
	want := map[deKey]int{{"d1", 1}: 3, {"d2", 1}: 3, {"d2", 4}: 5, {"d3", 1}: 1}
	if len(rows) != len(want) {
		t.Fatalf("doc_entities = %v, want %v", rows, want)
	}
	for k, v := range want {
		if rows[k] != v {
			t.Errorf("doc_entities[%v] = %d, want %d (all=%v)", k, rows[k], v, rows)
		}
	}

	// 链接 context 重写：mergee 展示名 → keeper 展示名；explicit 轨不动。
	var linkCtx string
	if err := db.QueryRowContext(ctx,
		`SELECT context FROM knowledge_links WHERE doc_id='d1' AND link_type='entity'`).Scan(&linkCtx); err != nil {
		t.Fatal(err)
	}
	if linkCtx != "AI,RAG" {
		t.Errorf("d1 entity link context = %q, want AI,RAG", linkCtx)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT context FROM knowledge_links WHERE doc_id='d3' AND link_type='entity'`).Scan(&linkCtx); err != nil {
		t.Fatal(err)
	}
	if linkCtx != "AI" {
		t.Errorf("d3 entity link context = %q, want AI", linkCtx)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT context FROM knowledge_links WHERE link_type='explicit'`).Scan(&linkCtx); err != nil {
		t.Fatal(err)
	}
	if linkCtx != "[[c]]" {
		t.Errorf("explicit link context = %q, want unchanged [[c]]", linkCtx)
	}

	// 冲突组同 norm：无需别名。
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entity_aliases`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("aliases = %d, want 0 (same-norm merge is redundant)", count)
	}

	// 唯一索引已建（幂等重跑不再失败）。
	var idx *string
	if err := db.QueryRowContext(ctx,
		`SELECT to_regclass('knowledge_entities_name_norm_key')::text`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx == nil || *idx == "" {
		t.Error("unique index knowledge_entities_name_norm_key missing")
	}

	// 幂等重跑。
	if err := ddlKnowledgeEntityGovernance(ctx, db, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("migration re-run: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("entities after re-run = %d, want 2", count)
	}
}

func TestDDLKnowledgeEntityGovernance_FreshDBSkips(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	// 空库无 knowledge 表（fresh 库由 EnsureKnowledgeSchema 建新形态），迁移整体跳过。
	if err := ddlKnowledgeEntityGovernance(context.Background(), db, nil, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("migration on empty schema: %v", err)
	}
	var regclass *string
	if err := db.QueryRowContext(context.Background(),
		`SELECT to_regclass('knowledge_entities')::text`).Scan(&regclass); err != nil {
		t.Fatal(err)
	}
	if regclass != nil && *regclass != "" {
		t.Error("knowledge_entities created by migration on fresh DB; EnsureKnowledgeSchema owns fresh shape")
	}
}
