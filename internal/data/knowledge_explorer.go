package data

// Vault 资源管理器持久化（P3）：轻量路径行（树构建）+ 已解析关联（关联区 R-3 来源标注）。
// 与 knowledge_links.go 同属派生索引纪律：随文档增删级联，查询只读。

import (
	"context"
	"database/sql"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

var (
	_ bizknowledge.DocumentPathReader = (*knowledgeRepo)(nil)
	_ bizknowledge.ResolvedLinkReader = (*knowledgeRepo)(nil)
)

// ListDocumentPaths 返回 vault 全部文档的轻量路径行（不含正文/向量，树聚合在 biz 内存完成）。
func (r *knowledgeRepo) ListDocumentPaths(ctx context.Context, collectionID string) ([]bizknowledge.DocumentPath, error) {
	acl, aclArgs := docRowVisibilityClause(ctx, 2)
	q := `SELECT id, rel_path, source, summary, tags, doc_type, status, size_bytes,
		        to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'), error_message
		 FROM knowledge_documents WHERE collection_id = $1 ` + acl
	args := append([]any{collectionID}, aclArgs...)
	rows, err := r.data.Postgres().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.DocumentPath
	for rows.Next() {
		var p bizknowledge.DocumentPath
		var tagsRaw []byte
		if err := rows.Scan(&p.ID, &p.RelPath, &p.Source, &p.Summary, &tagsRaw, &p.DocType,
			&p.Status, &p.SizeBytes, &p.UpdatedAt, &p.ErrorMessage); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		p.Tags = unmarshalTags(tagsRaw)
		out = append(out, p)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// ListResolvedLinks 列出文档关联（出向 + 入向），JOIN knowledge_documents 一次取回
// 对端 source/rel_path（禁 N+1）。target 已被级联清理，LEFT JOIN 仅为防御性兜底。
func (r *knowledgeRepo) ListResolvedLinks(ctx context.Context, collectionID, docID, linkType string) ([]bizknowledge.ResolvedLink, error) {
	q := `SELECT l.doc_id, l.target_doc_id, l.link_type, l.context,
	             s.source, s.rel_path, t.source, t.rel_path
	      FROM knowledge_links l
	      LEFT JOIN knowledge_documents s ON s.id = l.doc_id
	      LEFT JOIN knowledge_documents t ON t.id = l.target_doc_id
	      WHERE l.collection_id = $1 AND (l.doc_id = $2 OR l.target_doc_id = $2)`
	args := []any{collectionID, docID}
	if linkType != "" {
		q += ` AND l.link_type = $3`
		args = append(args, linkType)
	}
	q += ` ORDER BY l.link_type, l.id`
	rows, err := r.data.Postgres().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.ResolvedLink
	for rows.Next() {
		var srcID, dstID, lt, ctxStr string
		var srcSource, srcRel, dstSource, dstRel sql.NullString
		if err := rows.Scan(&srcID, &dstID, &lt, &ctxStr,
			&srcSource, &srcRel, &dstSource, &dstRel); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		rl := bizknowledge.ResolvedLink{LinkType: lt, Context: ctxStr}
		if srcID == docID {
			rl.Direction = "out"
			rl.TargetDocID = dstID
			rl.TargetSource = dstSource.String
			rl.TargetRelPath = dstRel.String
		} else {
			rl.Direction = "in"
			rl.TargetDocID = srcID
			rl.TargetSource = srcSource.String
			rl.TargetRelPath = srcRel.String
		}
		out = append(out, rl)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}
