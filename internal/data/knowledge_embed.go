package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"

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
		         COALESCE(NULLIF(visibility,''),'collection'), COALESCE(owner_user_id,''),
		         COALESCE(shared_with, '{}'::text[]),
		         chunk_strategy, chunk_size, chunk_overlap
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
// G-39（ADR-KN-20261021-G）：向量落侧表 upsert——同 (chunk_id, dim) 覆盖同代，
// 另批新 dim 共存；空向量跳过（等价旧 NULL pending 语义，由侧表 NOT EXISTS 表达）。
func (r *knowledgeRepo) UpdateChunkEmbeddings(ctx context.Context, docID string, vecs []bizknowledge.ChunkEmbedding) error {
	if len(vecs) == 0 {
		return nil
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding)
			 SELECT c.id, $2, c.collection_id, c.doc_id, $4, $1::vector
			 FROM knowledge_chunks c
			 WHERE c.id = $3 AND c.doc_id = $5
			 FOR UPDATE OF c
			 ON CONFLICT (chunk_id, dim) DO UPDATE
			 SET embedding = EXCLUDED.embedding, model = EXCLUDED.model`)
		if err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		defer stmt.Close()
		// 侧表 model 列溯源：取文档所属集合台账的 embedding_model（事务一次
		// 预查；无语义层集合落 ''，告警留痕与 insertChunksTx 同款契约）。
		// SELECT ... FOR UPDATE OF c：持 chunk 行锁与 MoveDocument 的 chunks
		// UPDATE 互斥串行——移动窗口内的陈读集合归属不会固化进侧行
		// （G-39 对抗评审 F2-B3）。
		var model string
		if err := tx.QueryRowContext(ctx,
			`SELECT kc.embedding_model FROM knowledge_documents d
			 JOIN knowledge_collections kc ON kc.id = d.collection_id
			 WHERE d.id = $1`, docID).Scan(&model); err != nil {
			r.lg.Warn("chunk side-embedding model lookup fell back to empty",
				loggateway.StepID("knowledge.chunk_side_model_skip"),
				loggateway.Err(err))
			model = ""
		}
		for _, v := range vecs {
			if len(v.Embedding) == 0 {
				continue
			}
			if _, err := stmt.ExecContext(ctx, pgvector.NewVector(v.Embedding), len(v.Embedding), v.ChunkID, model, docID); err != nil {
				return entErrToBizErr(err, "KNOWLEDGE")
			}
		}
		return nil
	})
}

// ListDocumentsPendingReembed 列出待重嵌入文档（B1 + G-39 按 dim 口径）：有正文、
// 非 indexing、且（无任何子块，或语义层集合存在欠缺当前 dim 向量的子块）。
// 按 created_at ASC（先入队先处理）。
// 返回完整文档（含 content_text，scanDocument 全字段扫描）——重嵌入以 content_text
// 为正文源，无需原始文件（与 ListEmbedDegradedDocuments 同理带正文）。
// 待嵌口径按子块（G-06）：父块语境行不落侧表，不计缺失——否则 parent_child
// 文档永久 pending、每轮修复全量重嵌空转。
// G-39（ADR-KN-20261021-G）：pending 判定从「主表 embedding IS NULL」改为
// 「侧表缺当前 dim 行」——换模型后旧 dim 向量共存不作废，缺失按新 dim 逐个
// 补齐；dim<=0（无语义层/未配 embedder）跳过缺维判定，只查子块缺失。
func (r *knowledgeRepo) ListDocumentsPendingReembed(ctx context.Context, collectionID string, dim int) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         content_text, organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         embed_fail_count, embed_last_tried,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(visibility,''),'collection'), COALESCE(owner_user_id,''),
		         COALESCE(shared_with, '{}'::text[]),
		         chunk_strategy, chunk_size, chunk_overlap
		  FROM knowledge_documents d
		  WHERE collection_id = $1
		    AND content_text <> ''
		    AND status <> 'indexing'
		    AND (
		      NOT EXISTS (SELECT 1 FROM knowledge_chunks c WHERE c.doc_id = d.id AND c.chunk_role = 'child')
		      OR (
		        EXISTS (
		          SELECT 1 FROM knowledge_collections kc
		          WHERE kc.id = d.collection_id AND kc.embedding_model <> ''
		        )
		        AND $2 > 0
		        AND EXISTS (
		          SELECT 1 FROM knowledge_chunks c
		          WHERE c.doc_id = d.id AND c.chunk_role = 'child'
		            AND NOT EXISTS (
		              SELECT 1 FROM chunk_embeddings ce
		              WHERE ce.chunk_id = c.id AND ce.dim = $2
		            )
		        )
		      )
		    )
		  ORDER BY created_at ASC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, collectionID, dim)
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
