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
	if _, err := repo.CommitIndexedDocument(ctx, colID, docID, chunks, 1, nil); err != nil {
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
	// T4 ⑩ 代际命名：父块定名 <docID>-v<ver>-p-<idx>（v1 摄取），子块父指针
	// 已被 resolveParentFinalID 重指到代际 ID，引用完整性由生产侧保持。
	parents, err := repo.ListChunksByIDs(ctx, colID, []string{docID + "-v1-p-0", docID + "-v1-p-404"})
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
		if ch.IsParent || ch.ParentChunkID != docID+"-v1-p-0" {
			t.Fatalf("child row shape broken: %+v", ch)
		}
	}

	// 同 doc 重提交：内容多重集恒等 → ② fast-path 零写（T4），collection 计数
	// 仍为 2。生产重嵌流（ReembedDocuments）先把文档状态拨回 indexing 再提交——
	// 测试镜像该时序以满足 CommitIndexedDocument 的 status CAS 守卫。
	if _, err := repo.data.rawDB.ExecContext(ctx, "UPDATE knowledge_documents SET status='indexing' WHERE id=$1", docID); err != nil {
		t.Fatalf("re-arm status before recommit: %v", err)
	}
	if _, err := repo.CommitIndexedDocument(ctx, colID, docID, chunks[:3], 0, nil); err != nil {
		t.Fatalf("recommit: %v", err)
	}
	col2, err := repo.GetCollection(ctx, colID)
	if err != nil {
		t.Fatalf("GetCollection after recommit: %v", err)
	}
	if col2.ChunkCount != 2 {
		t.Fatalf("recommit must keep child-only count, got %d", col2.ChunkCount)
	}

	// 待嵌口径按子块+侧表（G-39 判据：侧表缺当前 dim 才算 pending，主表
	// embedding NULL 不再是口径；G-06 修复：父块恒 NULL 不落侧表不拖累）：
	// 全部子块侧表已嵌入 → 不出现在 pending。
	pending, err := repo.ListDocumentsPendingReembed(ctx, colID, 3)
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("parent-child doc with all children embedded must not stay pending: %+v", pending)
	}
	// 同内容降级（nil 向量）重嵌：② fast-path 零写，kept 子块既有侧表行不被
	// nil 清洗，文档不因退化摄取无谓重嵌（⑪ 只补缺口不清存量）。生产意义：
	// embedding 断供期的重复摄取不销毁好向量（可用性优先）；真换 dim 才入队。
	degraded := append([]biz.KnowledgeChunk{}, chunks...)
	degraded[1].Embedding = nil
	if _, err := repo.data.rawDB.ExecContext(ctx, "UPDATE knowledge_documents SET status='indexing' WHERE id=$1", docID); err != nil {
		t.Fatalf("re-arm status before degraded recommit: %v", err)
	}
	if _, err := repo.CommitIndexedDocument(ctx, colID, docID, degraded, 0, nil); err != nil {
		t.Fatalf("recommit degraded: %v", err)
	}
	pending2, err := repo.ListDocumentsPendingReembed(ctx, colID, 3)
	if err != nil {
		t.Fatalf("ListDocumentsPendingReembed after degrade: %v", err)
	}
	if len(pending2) != 0 {
		t.Fatalf("degraded same-content recommit must keep side vectors and stay out of pending: %+v", pending2)
	}
	var sideN int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT count(*) FROM chunk_embeddings WHERE chunk_id=$1`, docID+"-v1-ch-0").Scan(&sideN); err != nil {
		t.Fatalf("side rows for kept child: %v", err)
	}
	if sideN != 1 {
		t.Fatalf("kept child side row must survive degraded recommit, got %d", sideN)
	}
}
