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
func (r *knowledgeRepo) CommitIndexedDocument(ctx context.Context, collectionID, docID string, chunks []biz.KnowledgeChunk, docDelta int, syncMeta *biz.KnowledgeDocumentSyncMeta) error {
	if len(chunks) > 0 {
		if err := r.assertChunkDimensions(ctx, chunks); err != nil {
			return err
		}
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 计数口径 = 子块（G-06）：父块语境行不进 chunk_count（documents 与
		// collections 两侧一致）。既有行 chunk_role 默认 'child' 天然兼容。
		var oldCount int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id = $1 AND chunk_role = 'child'`, docID).Scan(&oldCount); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE doc_id = $1`, docID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if err := r.insertChunksTx(ctx, tx, chunks); err != nil {
			return err
		}
		if syncMeta != nil {
			if err := r.updateDocumentSyncMetaTx(ctx, tx, docID, *syncMeta); err != nil {
				return err
			}
		}
		newCount := 0
		for _, ch := range chunks {
			if !ch.IsParent {
				newCount++
			}
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE knowledge_documents
			 SET status = 'indexed', error_message = '', chunk_count = $2, updated_at = NOW()
			 WHERE id = $1 AND status = 'indexing'`, docID, newCount)
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return bizknowledge.ErrDocumentStatusConflict
		}
		chunkDelta := newCount - oldCount
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_collections
			 SET document_count = document_count + $2,
			     chunk_count    = chunk_count    + $3,
			     updated_at     = NOW()
			 WHERE id = $1`, collectionID, docDelta, chunkDelta); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		return nil
	})
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

func (r *knowledgeRepo) assertChunkDimensions(ctx context.Context, chunks []biz.KnowledgeChunk) error {
	var expectedDim int
	err := r.data.Postgres().QueryRowContext(ctx,
		"SELECT dim FROM knowledge_collections WHERE id = $1", chunks[0].CollectionID).Scan(&expectedDim)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	for _, ch := range chunks {
		// G-06 父块行不嵌入（语境回填专用），跳过维度校验；dense 召回恒有
		// chunk_role='child' 谓词，误带向量也进不了召回。
		if ch.IsParent {
			continue
		}
		if expectedDim > 0 && len(ch.Embedding) != expectedDim {
			return apierror.BadRequest(apierror.DomainKnowledge, "embedding dimension mismatch: collection expects %d, chunk %q has %d", expectedDim, ch.ID, len(ch.Embedding))
		}
	}
	return nil
}

func (r *knowledgeRepo) insertChunksTx(ctx context.Context, tx *sql.Tx, chunks []biz.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, embedding, metadata, chunk_index, parent_chunk_id, chunk_role)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	defer stmt.Close()
	for _, ch := range chunks {
		meta := ch.MetadataJSON
		if meta == "" {
			meta = "{}"
		}
		// 无语义层 vault（R-4）/ 父子分块父块行（G-06）：空 embedding 写 NULL，
		// pgvector.NewVector(nil) 会生成非法 '[]'。
		var vec any
		if len(ch.Embedding) > 0 {
			vec = pgvector.NewVector(ch.Embedding)
		}
		role := "child"
		if ch.IsParent {
			role = "parent"
		}
		if _, err := stmt.ExecContext(ctx, ch.ID, ch.DocID, ch.CollectionID, ch.Content, vec, meta, ch.ChunkIndex, ch.ParentChunkID, role); err != nil {
			r.lg.Warn("chunk insert failed", loggateway.StepID("knowledge.chunk_insert_fail"), loggateway.Err(err))
			return entErrToBizErr(err, "knowledge")
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
