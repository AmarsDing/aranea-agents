package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/pgvector/pgvector-go"
)

// UpdateDocumentEmbedCircuit 回写文档熔断状态。failCount=0 表示复位（lastTried 清 NULL）。
func (r *knowledgeRepo) UpdateDocumentEmbedCircuit(ctx context.Context, id string, failCount int, lastTried time.Time) error {
	var tried any
	if !lastTried.IsZero() {
		tried = lastTried.UTC()
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET embed_fail_count = $2, embed_last_tried = $3, updated_at = NOW()
		 WHERE id = $1`, id, failCount, tried)
	return entErrToBizErr(err, "KNOWLEDGE")
}

// ListEmbedDegradedDocuments 列出熔断中（failCount>0）的文档，按退避起点升序
// （最久未尝试者优先）。含 content_text 供重分块回填向量。
func (r *knowledgeRepo) ListEmbedDegradedDocuments(ctx context.Context, collectionID string, limit int) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         content_text, organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         embed_fail_count, embed_last_tried,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(visibility,''),'collection'), COALESCE(owner_user_id,'')
		  FROM knowledge_documents
		  WHERE collection_id = $1 AND embed_fail_count > 0
		  ORDER BY embed_last_tried ASC NULLS FIRST
		  LIMIT $2`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, collectionID, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE")
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "KNOWLEDGE")
		}
		out = append(out, d)
	}
	return out, entErrToBizErr(rows.Err(), "KNOWLEDGE")
}

// UpdateChunkEmbeddings 恢复成功时按 chunk ID 精确寻址回填向量（ChunkIndex 源自
// splitter 元数据，不保证连续，禁止按位置映射）。docID 作为归属守卫防止跨文档误写。
func (r *knowledgeRepo) UpdateChunkEmbeddings(ctx context.Context, docID string, vecs []bizknowledge.ChunkEmbedding) error {
	if len(vecs) == 0 {
		return nil
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`UPDATE knowledge_chunks SET embedding = $1 WHERE id = $2 AND doc_id = $3`)
		if err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		defer stmt.Close()
		for _, v := range vecs {
			var vec any
			if len(v.Embedding) > 0 {
				vec = pgvector.NewVector(v.Embedding)
			}
			if _, err := stmt.ExecContext(ctx, vec, v.ChunkID, docID); err != nil {
				return entErrToBizErr(err, "KNOWLEDGE")
			}
		}
		return nil
	})
}

// ListDocumentsPendingReembed 列出待重嵌入文档（B1）：有正文、非 indexing、
// 且（chunks embedding IS NULL 或无任何 chunks）。按 created_at ASC（先入队先处理）。
// 返回完整文档（含 content_text，scanDocument 全字段扫描）——重嵌入以 content_text
// 为正文源，无需原始文件（与 ListEmbedDegradedDocuments 同理带正文）。
func (r *knowledgeRepo) ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         content_text, organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         embed_fail_count, embed_last_tried,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(visibility,''),'collection'), COALESCE(owner_user_id,'')
		  FROM knowledge_documents d
		  WHERE collection_id = $1
		    AND content_text <> ''
		    AND status <> 'indexing'
		    AND (
		      NOT EXISTS (SELECT 1 FROM knowledge_chunks c WHERE c.doc_id = d.id)
		      OR (
		        EXISTS (
		          SELECT 1 FROM knowledge_collections kc
		          WHERE kc.id = d.collection_id AND kc.embedding_model <> ''
		        )
		        AND EXISTS (
		          SELECT 1 FROM knowledge_chunks c
		          WHERE c.doc_id = d.id AND c.embedding IS NULL
		        )
		      )
		    )
		  ORDER BY created_at ASC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, collectionID)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, d)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}
