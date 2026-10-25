package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

func (r *knowledgeRepo) InsertChunks(ctx context.Context, chunks []biz.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	if err := r.assertChunkDimensions(ctx, chunks); err != nil {
		return err
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return r.insertChunksTx(ctx, tx, chunks)
	})
}

// CommitIndexedDocument replaces a document's chunks, CAS-marks it indexed,
// optionally writes vault sync meta, and adjusts collection counts in one TX.
// CommitIndexedDocument G-32 T4：十三步 diff-commit tx（执行体在
// knowledge_diff_commit.go；配对保留→档案化孤儿→代际命名新增→kept
// compare-and-skip→缺 dim 条件补齐→双条件版本行→guard/计数/syncMeta 原序）。
// 返回 ChunkDiffStats 供 T5 WS knowledge_chunks_diff 帧透出（决策 5⑬）。
func (r *knowledgeRepo) CommitIndexedDocument(ctx context.Context, collectionID, docID string, chunks []biz.KnowledgeChunk, docDelta int, syncMeta *biz.KnowledgeDocumentSyncMeta) (bizknowledge.ChunkDiffStats, error) {
	var stats bizknowledge.ChunkDiffStats
	if len(chunks) > 0 {
		if err := r.assertChunkDimensions(ctx, chunks); err != nil {
			return stats, err
		}
	}
	err := r.knowledgeExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		s, err := r.commitIndexedDocumentTx(ctx, tx, collectionID, docID, chunks, docDelta, syncMeta)
		stats = s
		return err
	})
	return stats, err
}

func (r *knowledgeRepo) updateDocumentSyncMetaTx(ctx context.Context, tx *sql.Tx, id string, meta biz.KnowledgeDocumentSyncMeta) error {
	tagsJSON, err := marshalTags(meta.Tags)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET content_hash = $2, summary = $3, summary_hash = $4, tags = $5, doc_type = $6, stale_at = NULL, updated_at = NOW()
		 WHERE id = $1`, id, meta.ContentHash, meta.Summary, meta.SummaryHash, tagsJSON, meta.DocType)
	return entErrToBizErr(err, "knowledge")
}

// assertChunkDimensions 批次统一校验（G-39，ADR-KN-20261021-G）：侧表多维度
// 共存后写入维度恒取 len(embedding)，不再读集合台账 dim（去掉 collections
// 往返）；只要求同一批次的非父块向量维度一致且 >0——批内混维是个体摄取
// 流程的完整性错误（同一文档不可能产两种维度），与跨批次多代共存不矛盾。
func (r *knowledgeRepo) assertChunkDimensions(_ context.Context, chunks []biz.KnowledgeChunk) error {
	var batchDim int
	for _, ch := range chunks {
		// G-06 父块行不嵌入（语境回填专用），跳过维度校验；dense 召回恒有
		// chunk_role='child' 谓词，误带向量也进不了召回。
		if ch.IsParent {
			continue
		}
		if len(ch.Embedding) == 0 {
			continue
		}
		if batchDim == 0 {
			batchDim = len(ch.Embedding)
			continue
		}
		if len(ch.Embedding) != batchDim {
			return apierror.BadRequest(apierror.DomainKnowledge, "embedding dimension mismatch: batch expects %d, chunk %q has %d", batchDim, ch.ID, len(ch.Embedding))
		}
	}
	return nil
}

func (r *knowledgeRepo) insertChunksTx(ctx context.Context, tx *sql.Tx, chunks []biz.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	// G-39（ADR-KN-20261021-G）：legacy embedding 列退出写路径（浸泡期列保留、
	// 恒 NULL），向量只落 chunk_embeddings 侧表。
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, metadata, chunk_index, parent_chunk_id, chunk_role)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	defer stmt.Close()
	// 侧表 upsert：同一 (chunk_id, dim) 重摄取覆盖同代向量；另批新 dim 合法
	// 共存（多代浸泡）。DELETE 旧块时侧表行经 FK 级联回收。
	sideStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (chunk_id, dim) DO UPDATE
		 SET embedding = EXCLUDED.embedding, model = EXCLUDED.model`)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	defer sideStmt.Close()
	// 侧表 model 列溯源用集合台账的 embedding_model（按集合惰性预查缓存；
	// 无语义层集合无台账语义 → 落 ''）。批内混集合时各取各自台账——
	// in-tree 调用方皆单集合，但 repo 端口不禁止混合批。单点口径见
	// chunkSideModelTx（与 diff-commit ⑪ 缺 dim 补齐同源）。
	models := make(map[string]string, 1)
	modelFor := func(colID string) string {
		if m, ok := models[colID]; ok {
			return m
		}
		m := r.chunkSideModelTx(ctx, tx, colID)
		models[colID] = m
		return m
	}
	for _, ch := range chunks {
		meta := ch.MetadataJSON
		if meta == "" {
			meta = "{}"
		}
		role := "child"
		if ch.IsParent {
			role = "parent"
		}
		if _, err := stmt.ExecContext(ctx, ch.ID, ch.DocID, ch.CollectionID, ch.Content, meta, ch.ChunkIndex, ch.ParentChunkID, role); err != nil {
			r.lg.Warn("chunk insert failed", loggateway.StepID("knowledge.chunk_insert_fail"), loggateway.Err(err))
			return entErrToBizErr(err, "knowledge")
		}
		// 无语义层 vault（R-4）/ 父子分块父块行（G-06）：空向量与父块行
		// 一律不落侧表（pending 语义由 ListDocumentsPendingReembed 侧表
		// NOT EXISTS 表达；父块带向量属带外构造，同口径拦截防索引污染）。
		if len(ch.Embedding) > 0 && !ch.IsParent {
			if _, err := sideStmt.ExecContext(ctx, ch.ID, len(ch.Embedding), ch.CollectionID, ch.DocID, modelFor(ch.CollectionID), pgvector.NewVector(ch.Embedding)); err != nil {
				r.lg.Warn("chunk side-embedding insert failed", loggateway.StepID("knowledge.chunk_side_embed_fail"), loggateway.Err(err))
				return entErrToBizErr(err, "knowledge")
			}
		}
	}
	return nil
}

func (r *knowledgeRepo) DeleteChunksByDocument(ctx context.Context, docID string) error {
	_, err := r.data.Postgres().ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE doc_id = $1`, docID)
	return entErrToBizErr(err, "knowledge")
}

// ListChunksByDocuments 按文档取前 limitPerDoc 个 chunk（chunk_index 升序）。
// Lazy GraphRAG 邻居扩展用：不走向量排序，避免二次 embed。空输入返回 nil。
// G-06：仅取子块（chunk_role='child'），父块语境行不进扩展集。
func (r *knowledgeRepo) ListChunksByDocuments(ctx context.Context, collectionID string, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error) {
	if strings.TrimSpace(collectionID) == "" || len(docIDs) == 0 || limitPerDoc <= 0 {
		return nil, nil
	}
	args := []any{collectionID, pq.Array(docIDs)}
	acl, aclArgs := docChunkVisibilityClause(ctx, "$1", len(args)+1)
	args = append(args, aclArgs...)
	limitPH := len(args) + 1
	args = append(args, limitPerDoc)
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index, parent_chunk_id, 0::float4 AS score
FROM (
  SELECT id, doc_id, collection_id, content, metadata, chunk_index, parent_chunk_id,
         ROW_NUMBER() OVER (PARTITION BY doc_id ORDER BY chunk_index) AS rn
  FROM knowledge_chunks
  WHERE collection_id = $1 AND chunk_role = 'child' AND doc_id = ANY($2)
  %s
) t
WHERE rn <= $%d
ORDER BY doc_id, chunk_index`, acl, limitPH)
	rows, err := r.data.PostgresRead().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []biz.KnowledgeChunk
	for rows.Next() {
		var ch biz.KnowledgeChunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.ParentChunkID, &ch.Score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, ch)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// ListChunksByIDs 按 chunk ID 批量取回行（G-06 语境回填的父块读取通道）。
// 无 chunk_role 谓词——调用方传入的正是父块 ID；ACL 仍走 docChunkVisibilityClause
// （父子同文档，可见性与子块召回一致）。空输入返回 nil。
func (r *knowledgeRepo) ListChunksByIDs(ctx context.Context, collectionID string, ids []string) ([]biz.KnowledgeChunk, error) {
	if strings.TrimSpace(collectionID) == "" || len(ids) == 0 {
		return nil, nil
	}
	args := []any{collectionID, pq.Array(ids)}
	acl, aclArgs := docChunkVisibilityClause(ctx, "$1", len(args)+1)
	args = append(args, aclArgs...)
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index, parent_chunk_id, 0::float4 AS score
FROM knowledge_chunks
WHERE collection_id = $1 AND id = ANY($2)
%s`, acl)
	rows, err := r.data.PostgresRead().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	return scanChunks(rows)
}

func scanChunks(rows *sql.Rows) ([]biz.KnowledgeChunk, error) {
	var out []biz.KnowledgeChunk
	for rows.Next() {
		var ch biz.KnowledgeChunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.ParentChunkID, &ch.Score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, ch)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}
