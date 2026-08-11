package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── B1 文档重嵌入：ListDocumentsPendingReembed 待重嵌入筛选 ──────────────────
// 维度对账（reconcileEmbeddingDim）将向量置 NULL 后，UI 上传文档无 vault_sync
// 自愈循环，由本筛选喂给 ReembedDocuments 重嵌入管线。

// TestKnowledgeRepo_ListDocumentsPendingReembed 覆盖筛选正确性：
// - 命中：chunks embedding IS NULL 的文档；无任何 chunks 但有 content_text 的文档
// - 排除：content_text='' 的文档；status='indexing' 的文档；embedding 非 NULL 的正常文档
// - 排序：created_at ASC（先入队先处理）
func TestKnowledgeRepo_ListDocumentsPendingReembed(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	// 同 setupDimReconcileRepo：pgvector 扩展在 public schema，钉单连接补 search_path。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	repo := &knowledgeRepo{data: d, lg: loggateway.NewNoop()}
	ctx := context.Background()

	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "kb", EmbeddingModel: "m", Dim: 3}); err != nil {
		t.Fatal(err)
	}

	mkdoc := func(id, contentText, status, createdAt string) {
		t.Helper()
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: id, CollectionID: "c1", Source: id + ".md", Status: status, ContentText: contentText,
		}); err != nil {
			t.Fatalf("create doc %s: %v", id, err)
		}
		// created_at 显式钉死，保证 ASC 排序断言确定（CreateDocument 用秒级时间戳，会同秒并列）。
		if _, err := db.ExecContext(ctx,
			`UPDATE knowledge_documents SET created_at = $2, updated_at = $2 WHERE id = $1`, id, createdAt); err != nil {
			t.Fatalf("pin created_at %s: %v", id, err)
		}
	}
	// nullChunk 模拟维度对账产物：chunk 行存在但 embedding 被置 NULL
	// （绕开 InsertChunks 的维度校验——NULL 向量非生产插入路径）。
	nullChunk := func(docID string) {
		t.Helper()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, embedding, metadata, chunk_index)
			 VALUES ($1, $2, 'c1', 'stale', NULL, '{}', 0)`, "nk-"+docID, docID); err != nil {
			t.Fatalf("insert null chunk for %s: %v", docID, err)
		}
	}

	mkdoc("d1-null-embed", "hello world", "indexed", "2026-08-12T02:00:00Z")
	nullChunk("d1-null-embed")
	mkdoc("d2-no-chunks", "fresh upload", "indexed", "2026-08-12T01:00:00Z")
	mkdoc("d3-indexing", "wip", "indexing", "2026-08-12T03:00:00Z")
	nullChunk("d3-indexing")
	mkdoc("d4-healthy", "fine", "indexed", "2026-08-12T04:00:00Z")
	mkdoc("d5-empty-content", "", "indexed", "2026-08-12T05:00:00Z")
	nullChunk("d5-empty-content")

	// 健康文档：正常 3 维向量（走 InsertChunks 维度校验路径）。
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "ok-d4", DocID: "d4-healthy", CollectionID: "c1", Content: "fine", Embedding: []float32{1, 0, 0},
	}}); err != nil {
		t.Fatalf("insert healthy chunk: %v", err)
	}

	got, err := repo.ListDocumentsPendingReembed(ctx, "c1")
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("pending count = %d, want 2: %+v", len(got), got)
	}
	// created_at ASC：d2 早于 d1。
	if got[0].ID != "d2-no-chunks" || got[1].ID != "d1-null-embed" {
		t.Fatalf("pending order = [%s %s], want [d2-no-chunks d1-null-embed]", got[0].ID, got[1].ID)
	}
	// 重嵌入以 content_text 为正文源：返回文档必须带该字段（summary 扫描器会丢弃）。
	if got[1].ContentText != "hello world" {
		t.Fatalf("pending doc content_text = %q, want %q（重嵌入依赖正文字段）", got[1].ContentText, "hello world")
	}
}
