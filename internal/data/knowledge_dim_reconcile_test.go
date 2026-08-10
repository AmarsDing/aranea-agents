package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 维度对账（2026-08-10 事故根修）：embedder 换模型后 knowledge_chunks.embedding ──
// 列类型仍是旧维度，CREATE TABLE IF NOT EXISTS 不会修正，所有新向量插入报
// "expected N dimensions"。EnsureKnowledgeSchema 必须在启动期幂等 reconcile。

func setupDimReconcileRepo(t *testing.T, dim int) (*knowledgeRepo, func(dim int) error) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	// 同 setupKnowledgeSearchRepo：pgvector 扩展在 public schema，钉单连接补 search_path。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	if err := EnsureKnowledgeSchema(context.Background(), db, dim); err != nil {
		t.Fatalf("ensure knowledge schema(dim=%d): %v", dim, err)
	}
	reensure := func(newDim int) error {
		return EnsureKnowledgeSchema(context.Background(), db, newDim)
	}
	return &knowledgeRepo{data: d, lg: loggateway.NewNoop()}, reensure
}

func embeddingColumnType(t *testing.T, repo *knowledgeRepo) string {
	t.Helper()
	var typ string
	if err := repo.data.Postgres().QueryRow(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		WHERE a.attrelid = 'knowledge_chunks'::regclass AND a.attname = 'embedding'`).Scan(&typ); err != nil {
		t.Fatalf("read embedding column type: %v", err)
	}
	return typ
}

// 换维度：列类型被 ALTER、旧向量作废置 NULL、受影响文档 content_hash 清空（触发重嵌入）、
// ivfflat 索引按新维度重建。
func TestEnsureKnowledgeSchema_ReconcilesEmbeddingDim(t *testing.T) {
	repo, reensure := setupDimReconcileRepo(t, 3)
	ctx := context.Background()

	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault", EmbeddingModel: "m", Dim: 3, RootPath: "/vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", Source: "a.md", RelPath: "a.md", Status: "indexed", ContentHash: "hash-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "k1", DocID: "d1", CollectionID: "c1", Content: "hello", Embedding: []float32{1, 0, 0},
	}}); err != nil {
		t.Fatal(err)
	}

	// embedder 换模型：3 维 → 7 维。
	if err := reensure(7); err != nil {
		t.Fatalf("re-ensure with new dim: %v", err)
	}

	if typ := embeddingColumnType(t, repo); typ != "vector(7)" {
		t.Fatalf("embedding column type = %s, want vector(7)", typ)
	}
	// 有语义层集合的 dim 快照同步（否则应用层校验继续拒绝新维度插入 = 死库）。
	col, err := repo.GetCollection(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if col.Dim != 7 {
		t.Fatalf("collection dim snapshot = %d, want reconciled to 7", col.Dim)
	}
	var nonNull int
	if err := repo.data.Postgres().QueryRow(`SELECT COUNT(*) FROM knowledge_chunks WHERE embedding IS NOT NULL`).Scan(&nonNull); err != nil {
		t.Fatal(err)
	}
	if nonNull != 0 {
		t.Fatalf("stale embeddings must be nulled on dim change, got %d non-null", nonNull)
	}
	doc, err := repo.GetDocument(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.ContentHash != "" {
		t.Fatalf("affected document content_hash = %q, want empty (force re-embed)", doc.ContentHash)
	}
	// 索引存在且可用：新维度向量可插入并检索。
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "k2", DocID: "d1", CollectionID: "c1", Content: "world", Embedding: []float32{1, 0, 0, 0, 0, 0, 0},
	}}); err != nil {
		t.Fatalf("insert %d-dim chunk after reconcile: %v", 7, err)
	}
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 5}, []float32{1, 0, 0, 0, 0, 0, 0})
	if err != nil {
		t.Fatalf("search after reconcile: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k2" {
		t.Fatalf("search after reconcile = %+v, want only k2 (null-embedding rows excluded)", got)
	}
}

// 维度一致：零动作，存量向量与文档标记原样保留。
func TestEnsureKnowledgeSchema_SameDim_NoOp(t *testing.T) {
	repo, reensure := setupDimReconcileRepo(t, 3)
	ctx := context.Background()

	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault", Dim: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", Source: "a.md", Status: "indexed", ContentHash: "hash-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "k1", DocID: "d1", CollectionID: "c1", Content: "hello", Embedding: []float32{1, 0, 0},
	}}); err != nil {
		t.Fatal(err)
	}

	if err := reensure(3); err != nil {
		t.Fatalf("re-ensure same dim: %v", err)
	}

	if typ := embeddingColumnType(t, repo); typ != "vector(3)" {
		t.Fatalf("embedding column type = %s, want vector(3)", typ)
	}
	var nonNull int
	if err := repo.data.Postgres().QueryRow(`SELECT COUNT(*) FROM knowledge_chunks WHERE embedding IS NOT NULL`).Scan(&nonNull); err != nil {
		t.Fatal(err)
	}
	if nonNull != 1 {
		t.Fatalf("same-dim re-ensure must keep embeddings, got %d non-null (want 1)", nonNull)
	}
	doc, err := repo.GetDocument(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.ContentHash != "hash-1" {
		t.Fatalf("same-dim re-ensure must keep content_hash, got %q", doc.ContentHash)
	}
}
