package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// G-06 父子分块数据面 PG 契约（手写 DDL/孪生库 Σ 夹具同款夹具族）：
//  1. 20261283 列经 EnsureKnowledgeSchema 落位（连跑两次幂等，DB-N6）；
//  2. CommitIndexedDocument：父子混合块同行的 DELETE+INSERT，chunk_count 口径 = 子块
//     （父块语境行不进 documents.chunk_count 与 collections.chunk_count）；
//  3. ListChunksByIDs：父块语境回填端口——按 ID 批量取回（含 parent 行），未知 ID 静默缺席；
//  4. ListChunksByDocuments：检索路径剔除 parent 行（防御纵深 chunk_role = 'child'）。

func setupParentChildRepo(t *testing.T) (context.Context, *knowledgeRepo, string, string) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	// 幂等复跑（ALTER ... IF NOT EXISTS / CREATE INDEX IF NOT EXISTS 双跑不炸，DB-N6）。
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema (idempotent re-run): %v", err)
	}
	repo := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), dialect: DialectPostgres},
		loggateway.NewNoop())
	ctx := context.Background()
	col, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "pc-col", Name: "pc", EmbeddingModel: "test", Dim: 3,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	doc, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "pc-doc", CollectionID: col.ID, Source: "pc.md", MimeType: "text/markdown",
		Status: "indexing", ContentText: "物料清单\n物料A 仓储区一层 编码MAT-A\n物料B 恒温库 编码MAT-B",
		ChunkStrategy: "parent_child", ChunkSize: 8, ChunkOverlap: 2,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	return ctx, repo.(*knowledgeRepo), col.ID, doc.ID
}

// 摄取形状列落库 + reembed 回读保形（G-06 修复重嵌恒 char 的读取侧）。
func TestKnowledgeRepo_DocumentChunkShape_RoundTrip(t *testing.T) {
	ctx, repo, _, docID := setupParentChildRepo(t)
	got, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.ChunkStrategy != "parent_child" || got.ChunkSize != 8 || got.ChunkOverlap != 2 {
		t.Fatalf("chunk shape not round-tripped: %+v", got)
	}
	// 夹具建文档为 indexing（CommitIndexedDocument 的 WHERE 守卫需要）；径到
	// pending-reembed 口径前先推进到 pending——夹具共享，不能在建具处改状态（对抗评审 F3）。
	if _, err := repo.data.rawDB.ExecContext(ctx, "UPDATE knowledge_documents SET status='pending' WHERE id=$1", docID); err != nil {
		t.Fatalf("advance doc status: %v", err)
	}
	pending, err := repo.ListDocumentsPendingReembed(ctx, got.CollectionID, 3)
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed: %v", err)
	}
	if len(pending) != 1 || pending[0].ChunkStrategy != "parent_child" || pending[0].ChunkSize != 8 {
		t.Fatalf("pending reembed must carry stored shape: %+v", pending)
	}
}

// 父子混合提交的计数口径与回填端口。
func TestKnowledgeRepo_ParentChildCommitAndFetch(t *testing.T) {
	ctx, repo, colID, docID := setupParentChildRepo(t)

	emb := []float32{0.1, 0.2, 0.3}
	chunks := []biz.KnowledgeChunk{
		{ID: docID + "-p-0", DocID: docID, CollectionID: colID, Content: "父块：物料A+B", ChunkIndex: 0, IsParent: true},
		{ID: docID + "-ch-0", DocID: docID, CollectionID: colID, Content: "物料A 仓储区一层", ChunkIndex: 0, ParentChunkID: docID + "-p-0", Embedding: emb},
		{ID: docID + "-ch-1", DocID: docID, CollectionID: colID, Content: "物料B 恒温库", ChunkIndex: 1, ParentChunkID: docID + "-p-0", Embedding: emb},
	}
	if err := repo.CommitIndexedDocument(ctx, colID, docID, chunks, 1, nil); err != nil {
		t.Fatalf("commit parent-child chunks: %v", err)
	}

	// 计数口径 = 子块：documents/collections 两侧均为 2（父块不计入）。
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.ChunkCount != 2 {
		t.Fatalf("documents.chunk_count must count children only, got %d", doc.ChunkCount)
	}
	col, err := repo.GetCollection(ctx, colID)
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if col.ChunkCount != 2 {
		t.Fatalf("collections.chunk_count must count children only, got %d", col.ChunkCount)
	}

	// ListChunksByIDs：父块回填端口——取回父块正文，未知 ID 静默缺席。
	parents, err := repo.ListChunksByIDs(ctx, colID, []string{docID + "-p-0", docID + "-p-404"})
	if err != nil {
		t.Fatalf("ListChunksByIDs: %v", err)
	}
	if len(parents) != 1 || parents[0].Content != "父块：物料A+B" {
		t.Fatalf("parent fetch broken: %+v", parents)
	}
	if parents[0].ParentChunkID != "" {
		t.Fatalf("parent row must not itself carry parent ref: %+v", parents[0])
	}

	// ListChunksByDocuments：检索面剔除 parent 行（仅 2 子块，子块带 parent_chunk_id）。
	visible, err := repo.ListChunksByDocuments(ctx, colID, []string{docID}, 10)
	if err != nil {
		t.Fatalf("ListChunksByDocuments: %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("retrieval surface must exclude parent rows, got %d: %+v", len(visible), visible)
	}
	for _, ch := range visible {
		if ch.IsParent || ch.ParentChunkID != docID+"-p-0" {
			t.Fatalf("child row shape broken: %+v", ch)
		}
	}

	// 同 doc 重提交（重嵌口径）：旧行整删重插，collection 计数仍为 2（delta 以子块计）。
	// 生产重嵌流（ReembedDocuments）先把文档状态拨回 indexing 再提交——测试镜像该
	// 时序以满足 CommitIndexedDocument 的 status CAS 守卫（doc 已是 indexed）。
	if _, err := repo.data.rawDB.ExecContext(ctx, "UPDATE knowledge_documents SET status='indexing' WHERE id=$1", docID); err != nil {
		t.Fatalf("re-arm status before recommit: %v", err)
	}
	if err := repo.CommitIndexedDocument(ctx, colID, docID, chunks[:3], 0, nil); err != nil {
		t.Fatalf("recommit: %v", err)
	}
	col2, err := repo.GetCollection(ctx, colID)
	if err != nil {
		t.Fatalf("GetCollection after recommit: %v", err)
	}
	if col2.ChunkCount != 2 {
		t.Fatalf("recommit must keep child-only count, got %d", col2.ChunkCount)
	}

	// 待嵌口径按子块（G-06 修复：父块恒 embedding NULL 不得拖文档永久 pending）：
	// 全部子块已嵌入 → 不出现在 pending；存在 NULL 嵌入子块 → 出现。
	pending, err := repo.ListDocumentsPendingReembed(ctx, colID, 3)
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("parent-child doc with all children embedded must not stay pending: %+v", pending)
	}
	degraded := append([]biz.KnowledgeChunk{}, chunks...)
	degraded[1].Embedding = nil
	// 同上：镜像生产重嵌时序，先拨回 indexing 再过 CAS。
	if _, err := repo.data.rawDB.ExecContext(ctx, "UPDATE knowledge_documents SET status='indexing' WHERE id=$1", docID); err != nil {
		t.Fatalf("re-arm status before degraded recommit: %v", err)
	}
	if err := repo.CommitIndexedDocument(ctx, colID, docID, degraded, 0, nil); err != nil {
		t.Fatalf("recommit degraded: %v", err)
	}
	pending2, err := repo.ListDocumentsPendingReembed(ctx, colID, 3)
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed after degrade: %v", err)
	}
	if len(pending2) != 1 || pending2[0].ID != docID {
		t.Fatalf("doc with NULL-embedding child must be pending: %+v", pending2)
	}
}
