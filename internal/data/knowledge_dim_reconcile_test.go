package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── G-39 多维度向量共存层（ADR-KN-20261021-G）：reconcileEmbeddingDim（08-10
// 事故根修的 wipe 对账）已退役——换模型不再 ALTER 主表列、不作废旧 dim 向量，
// 改由 chunk_embeddings 侧表 (chunk_id, dim) 分列共存承载；Ensure 启动尾部
// （ensureMultiDimEmbeddingLayer）只做幂等回填 + 台账 dim 对齐 + per-dim
// partial HNSW 增量注册，全程零 wipe。这两个用例锁定「G-39 的 dim flip 安全」
// 语义：正是 08-10 事故的反面。

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

// 换维度（3→7）：零 wipe——legacy 主表列保持初始 typmod 浸泡不动、侧表旧 dim
// 行保留、文档 content_hash 不清（缺失按 ListDocumentsPendingReembed 逐 dim
// 补齐，自愈非全量作废）；集合台账 dim 只作账本更新为 7；新 dim 摄取即时合法
// （per-dim partial HNSW 随 Ensure 预注册），两代向量共存检索互不串桶。
func TestEnsureKnowledgeSchema_DimFlipPreservesVectors(t *testing.T) {
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

	// legacy 主表列浸泡保留：列型维持初始 vector(3)（TK-G39-01 才 DROP），
	// 行恒 NULL（写路径早已脱离主列）。零 ALTER、零 wipe。
	if typ := embeddingColumnType(t, repo); typ != "vector(3)" {
		t.Fatalf("legacy embedding column must stay at initial typmod under soak, got %s", typ)
	}
	// 集合台账 dim 对齐为账本语义（不再约束写入维度合法性）。
	col, err := repo.GetCollection(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if col.Dim != 7 {
		t.Fatalf("collection dim ledger = %d, want aligned to 7", col.Dim)
	}
	// 旧 dim 侧表行保留（共存，不作废）。
	var sideOld int
	if err := repo.data.Postgres().QueryRow(`
		SELECT COUNT(*) FROM chunk_embeddings WHERE chunk_id = 'k1' AND dim = 3`).Scan(&sideOld); err != nil {
		t.Fatal(err)
	}
	if sideOld != 1 {
		t.Fatalf("old-dim side row must be preserved on dim flip, got %d", sideOld)
	}
	// content_hash 不清空：自愈走 pending-reembed 按 dim 逐文档补齐，不再全量迫重嵌。
	doc, err := repo.GetDocument(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.ContentHash != "hash-1" {
		t.Fatalf("content_hash must be preserved (heal via pending-reembed), got %q", doc.ContentHash)
	}
	// 新 dim 摄取即时合法（curDim=7 的 per-dim partial HNSW 已预注册）。
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "k2", DocID: "d1", CollectionID: "c1", Content: "world", Embedding: []float32{1, 0, 0, 0, 0, 0, 0},
	}}); err != nil {
		t.Fatalf("insert %d-dim chunk after dim flip: %v", 7, err)
	}
	// 两代共存：dim 7 查询只中 k2；dim 3 查询只中 k1（桶定，无 22P02）。
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 5}, []float32{1, 0, 0, 0, 0, 0, 0})
	if err != nil {
		t.Fatalf("search dim 7 after flip: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k2" {
		t.Fatalf("dim-7 search = %+v, want only k2 (dim-3 bucket untouched)", got)
	}
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 5}, []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("search dim 3 after flip: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k1" {
		t.Fatalf("dim-3 search = %+v, want only k1 (old generation still searchable)", got)
	}
}

// 维度一致复跑：零动作——侧表行、legacy 列、文档标记原样保留（回填 NOT EXISTS
// 反连接 + ON CONFLICT 幂等，台账 UPDATE 命中 dim = $1 前置谓词零行）。
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
		t.Fatalf("legacy embedding column type = %s, want vector(3) (soaked untouched)", typ)
	}
	var sideRows int
	if err := repo.data.Postgres().QueryRow(`
		SELECT COUNT(*) FROM chunk_embeddings WHERE chunk_id = 'k1' AND dim = 3`).Scan(&sideRows); err != nil {
		t.Fatal(err)
	}
	if sideRows != 1 {
		t.Fatalf("same-dim re-ensure must keep side-table rows, got %d (want 1)", sideRows)
	}
	doc, err := repo.GetDocument(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.ContentHash != "hash-1" {
		t.Fatalf("same-dim re-ensure must keep content_hash, got %q", doc.ContentHash)
	}
	// 检索侧零回归：dim-3 桶照常命中。
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 5}, []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("search after same-dim re-ensure: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k1" {
		t.Fatalf("search after same-dim re-ensure = %+v, want k1", got)
	}
}

func knowledgeEmbeddingIndexAM(t *testing.T, repo *knowledgeRepo, name string) string {
	t.Helper()
	var am string
	err := repo.data.Postgres().QueryRow(`
		SELECT am.amname
		FROM pg_class c
		JOIN pg_am am ON am.oid = c.relam
		WHERE c.relname = $1`, name).Scan(&am)
	if err != nil {
		t.Fatalf("index %s access method: %v", name, err)
	}
	return am
}

func TestEnsureKnowledgeSchema_UsesHNSWIndex(t *testing.T) {
	repo, _ := setupDimReconcileRepo(t, 3)
	if got := knowledgeEmbeddingIndexAM(t, repo, knowledgeEmbeddingHNSWIndex); got != "hnsw" {
		t.Fatalf("embedding index AM = %s, want hnsw", got)
	}
	var ivf int
	if err := repo.data.Postgres().QueryRow(`
		SELECT COUNT(*) FROM pg_class WHERE relname = $1`, knowledgeEmbeddingIVFFlatIndex).Scan(&ivf); err != nil {
		t.Fatal(err)
	}
	if ivf != 0 {
		t.Fatal("legacy ivfflat index must be dropped")
	}
}
