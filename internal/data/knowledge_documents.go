package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

func (r *knowledgeRepo) CreateDocument(ctx context.Context, d biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, err := marshalTags(d.Tags)
	if err != nil {
		return biz.KnowledgeDocument{}, entErrToBizErr(err, "knowledge")
	}
	q := `INSERT INTO knowledge_documents
		(id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message, content_text, organized, asset_uri,
		 rel_path, content_hash, summary, summary_hash, tags, doc_type, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,0,$6,'',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
		RETURNING id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
	          content_text, organized, asset_uri,
	          rel_path, content_hash, summary, summary_hash, tags, doc_type,
	          embed_fail_count, embed_last_tried,
	          to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	          COALESCE(NULLIF(visibility,''),'collection'), COALESCE(owner_user_id,'')`
	row := r.data.Postgres().QueryRowContext(ctx, q, d.ID, d.CollectionID, d.Source, d.MimeType, d.SizeBytes, d.Status,
		d.ContentText, d.Organized, d.AssetURI,
		d.RelPath, d.ContentHash, d.Summary, d.SummaryHash, tagsJSON, d.DocType, now)
	return scanDocument(row)
}

// GetDocumentByRelPath 按 vault 相对路径寻址文档（Vault 同步用，rel_path 唯一）。
func (r *knowledgeRepo) GetDocumentByRelPath(ctx context.Context, collectionID, relPath string) (biz.KnowledgeDocument, error) {
	q := `SELECT d.id, d.collection_id, d.source, d.mime_type, d.size_bytes, d.chunk_count, d.status, d.error_message,
		         d.content_text, d.organized, d.asset_uri,
		         d.rel_path, d.content_hash, d.summary, d.summary_hash, d.tags, d.doc_type,
		         d.embed_fail_count, d.embed_last_tried,
		         to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(d.visibility,''),'collection'), COALESCE(d.owner_user_id,'')
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.collection_id = $1 AND d.rel_path = $2`
	args := []any{collectionID, relPath}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $3 OR c.workspace = '')`
		args = append(args, ws)
	}
	doc, err := scanDocument(r.data.Postgres().QueryRowContext(ctx, q, args...))
	if err != nil {

		return doc, entErrToBizErr(err, "KNOWLEDGE")
	}
	return doc, nil
}

// GetDocumentByContentHash 按正文 hash 寻址（入库去重）。空 hash 不查库。
func (r *knowledgeRepo) GetDocumentByContentHash(ctx context.Context, collectionID, contentHash string) (biz.KnowledgeDocument, error) {
	if strings.TrimSpace(contentHash) == "" {
		return biz.KnowledgeDocument{}, apierror.NotFound(apierror.DomainKnowledge, "document hash not found")
	}
	q := `SELECT d.id, d.collection_id, d.source, d.mime_type, d.size_bytes, d.chunk_count, d.status, d.error_message,
		         d.content_text, d.organized, d.asset_uri,
		         d.rel_path, d.content_hash, d.summary, d.summary_hash, d.tags, d.doc_type,
		         d.embed_fail_count, d.embed_last_tried,
		         to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(d.visibility,''),'collection'), COALESCE(d.owner_user_id,'')
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.collection_id = $1 AND d.content_hash = $2`
	args := []any{collectionID, contentHash}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $3 OR c.workspace = '')`
		args = append(args, ws)
	}
	q += ` ORDER BY d.created_at ASC LIMIT 1`
	doc, err := scanDocument(r.data.Postgres().QueryRowContext(ctx, q, args...))
	if err != nil {
		return doc, entErrToBizErr(err, "KNOWLEDGE")
	}
	return doc, nil
}

// ListDocumentNames 批量解析文档显示名（SP1-E DocNameReader）：rel_path 优先，
// 空 rel_path 回 source；未知 id 缺席。空入参短路空 map（避免空数组查询）。
func (r *knowledgeRepo) ListDocumentNames(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, COALESCE(NULLIF(rel_path, ''), source) FROM knowledge_documents WHERE id = ANY($1)`,
		pq.Array(ids))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}

// UpdateDocumentRelPath 文件移动/重命名时更新镜像路径（保留文档身份与索引）。
func (r *knowledgeRepo) UpdateDocumentRelPath(ctx context.Context, id, newRelPath string) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET rel_path = $2, updated_at = NOW() WHERE id = $1`, id, newRelPath)
	return entErrToBizErr(err, "knowledge")
}

// UpdateDocumentSyncMeta 文件内容变更时回写同步元数据（Vault 同步 modified 事件，P1-3）。
func (r *knowledgeRepo) UpdateDocumentSyncMeta(ctx context.Context, id string, meta biz.KnowledgeDocumentSyncMeta) error {
	tagsJSON, err := marshalTags(meta.Tags)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	_, err = r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET content_hash = $2, summary = $3, summary_hash = $4, tags = $5, doc_type = $6, stale_at = NULL, updated_at = NOW()
		 WHERE id = $1`, id, meta.ContentHash, meta.Summary, meta.SummaryHash, tagsJSON, meta.DocType)
	return entErrToBizErr(err, "knowledge")
}

func (r *knowledgeRepo) GetDocument(ctx context.Context, id string) (biz.KnowledgeDocument, error) {

	q := `SELECT d.id, d.collection_id, d.source, d.mime_type, d.size_bytes, d.chunk_count, d.status, d.error_message,
		         d.content_text, d.organized, d.asset_uri,
		         d.rel_path, d.content_hash, d.summary, d.summary_hash, d.tags, d.doc_type,
		         d.embed_fail_count, d.embed_last_tried,
		         to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         COALESCE(NULLIF(d.visibility,''),'collection'), COALESCE(d.owner_user_id,'')
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.id = $1`
	args := []any{id}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $2 OR c.workspace = '')`
		args = append(args, ws)
	}
	doc, err := scanDocument(r.data.Postgres().QueryRowContext(ctx, q, args...))
	if err != nil {
		return doc, entErrToBizErr(err, "KNOWLEDGE")
	}
	return doc, nil
}

func (r *knowledgeRepo) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET status = $2, error_message = $3, chunk_count = $4, updated_at = NOW()
		 WHERE id = $1`, id, status, errMsg, chunkCount)
	return entErrToBizErr(err, "knowledge")
}

// UpdateDocumentContent 回写文档正文与整理标记（Phase 9 图片异步提取完成后调用）。
// stale_at 清 NULL（P1-c）：内容变更即新鲜，陈旧降权复活。
func (r *knowledgeRepo) UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET content_text = $2, organized = $3, stale_at = NULL, updated_at = NOW()
		 WHERE id = $1`, id, contentText, organized)
	return entErrToBizErr(err, "knowledge")
}

func (r *knowledgeRepo) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]biz.KnowledgeDocument, int, error) {
	countACL, countACLArgs := docRowVisibilityClause(ctx, 2)
	countSQL := `SELECT COUNT(*) FROM knowledge_documents WHERE (collection_id = $1 OR $1 = '') ` + countACL
	countArgs := append([]any{collectionID}, countACLArgs...)
	var total int
	if err := r.data.Postgres().QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, entErrToBizErr(err, "knowledge")
	}
	listACL, listACLArgs := docRowVisibilityClause(ctx, 4)
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents WHERE (collection_id = $1 OR $1 = '') ` + listACL + `
		  ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	args := append([]any{collectionID, limit, offset}, listACLArgs...)
	rows, err := r.data.Postgres().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocumentSummary(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "knowledge")
		}
		out = append(out, d)
	}
	return out, total, entErrToBizErr(rows.Err(), "knowledge")
}

// DeleteDocument removes a document, lets the FK cascade drop its chunks
// (links/doc_entities/relation_state/fact_version likewise cascade), explicitly
// deletes the FK-less knowledge_access_log rows, and atomically adjusts the
// owning collection's cached counters (DAT-02 / REV-B).
//
// Counter adjustment rules:
//   - document_count is only decremented when the document was successfully
//     indexed (status = "indexed"). For pending/indexing/error documents the
//     collection counter was never incremented, so decrementing would produce
//     drift.
//   - chunk_count is decremented by the document's chunk_count regardless of
//     status, but GREATEST guards against going below 0. (A partially-indexed
//     document that failed mid-way may have some chunks already counted.)
//
// All statements run in one transaction to avoid any partial-update window.
func (r *knowledgeRepo) DeleteDocument(ctx context.Context, id string) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var collectionID string
		var chunkCount int
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT collection_id, chunk_count, status FROM knowledge_documents WHERE id = $1`, id).
			Scan(&collectionID, &chunkCount, &status)
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil
		}
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_access_log WHERE doc_id = $1`, id); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id = $1`, id); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if collectionID != "" {
			docDelta := 0
			if status == "indexed" {
				docDelta = 1
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE knowledge_collections
				 SET document_count = GREATEST(document_count - $2, 0),
				     chunk_count    = GREATEST(chunk_count    - $3, 0),
				     updated_at     = NOW()
				 WHERE id = $1`, collectionID, docDelta, chunkCount); err != nil {
				r.lg.Warn("delete document counter update failed", loggateway.StepID("knowledge.counter_drift"), loggateway.Err(err))
				return entErrToBizErr(err, "knowledge")
			}
		}
		return nil
	})
}

func scanDocument(row scannable) (biz.KnowledgeDocument, error) {
	var d biz.KnowledgeDocument
	var tagsRaw []byte
	var embedLastTried sql.NullTime
	err := row.Scan(&d.ID, &d.CollectionID, &d.Source, &d.MimeType, &d.SizeBytes,
		&d.ChunkCount, &d.Status, &d.ErrorMessage, &d.ContentText, &d.Organized, &d.AssetURI,
		&d.RelPath, &d.ContentHash, &d.Summary, &d.SummaryHash, &tagsRaw, &d.DocType,
		&d.EmbedFailCount, &embedLastTried,
		&d.CreatedAt, &d.UpdatedAt, &d.Visibility, &d.OwnerUserID)
	if err != nil {
		return d, entErrToBizErr(err, "knowledge")
	}
	d.Tags = unmarshalTags(tagsRaw)
	if embedLastTried.Valid {
		d.EmbedLastTried = embedLastTried.Time
	}
	return d, nil
}

// scanDocumentSummary 用于列表查询：不取 content_text 大字段（避免列表带宽放大）。
func scanDocumentSummary(row scannable) (biz.KnowledgeDocument, error) {
	var d biz.KnowledgeDocument
	var tagsRaw []byte
	err := row.Scan(&d.ID, &d.CollectionID, &d.Source, &d.MimeType, &d.SizeBytes,
		&d.ChunkCount, &d.Status, &d.ErrorMessage, &d.Organized, &d.AssetURI,
		&d.RelPath, &d.ContentHash, &d.Summary, &d.SummaryHash, &tagsRaw, &d.DocType,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, entErrToBizErr(err, "knowledge")
	}
	d.Tags = unmarshalTags(tagsRaw)
	return d, nil
}
