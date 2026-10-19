package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// MoveDocument moves a document (and its chunks) to another collection in one
// transaction, keeping both collections' cached counters in sync (US-14).
//
// 附表随迁（治理归属修复）：knowledge_links 仅迁源端（doc_id = 本文档）——
// 边 collection_id 语义即「源文档集合」，target 端不动（跨集合边保留在源
// 集合治理域）；access_log/doc_entities/relation_state/fact_version 按
// doc_id 随迁。不迁则旧集合 decay/hub/distill 把已移走文档算入、新集合
// 治理扫不到（治理黑洞）。
//
// Counter rules mirror DeleteDocument: document_count shifts only when the
// document was successfully indexed (pending/indexing/error docs were never
// counted); chunk_count shifts by the document's recorded chunk_count,
// GREATEST-guarded on the source side against drift below zero.
func (r *knowledgeRepo) MoveDocument(ctx context.Context, id, targetCollectionID string) (biz.KnowledgeDocument, error) {
	err := r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var sourceCollectionID string
		var chunkCount int
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT collection_id, chunk_count, status FROM knowledge_documents WHERE id = $1`, id).
			Scan(&sourceCollectionID, &chunkCount, &status)
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if sourceCollectionID == targetCollectionID {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_documents SET collection_id = $2, updated_at = NOW() WHERE id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_chunks SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_links SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_access_log SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_doc_entities SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_relation_state SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_fact_version SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		docDelta := 0
		if status == "indexed" {
			docDelta = 1
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_collections
			 SET document_count = GREATEST(document_count - $2, 0),
			     chunk_count    = GREATEST(chunk_count    - $3, 0),
			     updated_at     = NOW()
			 WHERE id = $1`, sourceCollectionID, docDelta, chunkCount); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_collections
			 SET document_count = document_count + $2,
			     chunk_count    = chunk_count    + $3,
			     updated_at     = NOW()
			 WHERE id = $1`, targetCollectionID, docDelta, chunkCount); err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		return nil
	})
	if err != nil {
		return biz.KnowledgeDocument{}, entErrToBizErr(err, "knowledge")
	}
	return r.GetDocument(ctx, id)
}

// pathPrefixClause 生成 G3-B7 搜索范围过滤子查询：仅命中文档 rel_path 位于
// "<prefix>/" 下的 chunks（目录边界语义，防 "note" 误中 "notes/"）。首尾斜杠
// 归一；LIKE 通配符（% _ \）按字面值转义。返回空串 = 不过滤。
// collectionPH 是外层查询中 collection_id 的占位符（如 "$2"），argIdx 是本
// 子查询 LIKE 参数的占位序号。
func pathPrefixClause(prefix, collectionPH string, argIdx int) (clause string, arg any) {
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	if p == "" {
		return "", nil
	}
	return fmt.Sprintf(`AND doc_id IN (SELECT id FROM knowledge_documents WHERE collection_id = %s AND rel_path LIKE $%d ESCAPE '\')`, collectionPH, argIdx), escapeLikePrefix(p) + "/%"
}

func escapeLikePrefix(p string) string {
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range p {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pathExcludeClause 检索排除子查询（词条优先写回）：rel_path 以任一前缀开头
// 的文档不命中。字面前缀语义（不补目录边界），argIdx 为首个 LIKE 占位序号。
func pathExcludeClause(prefixes []string, collectionPH string, argIdx int) (clause string, args []any) {
	var likes []string
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		likes = append(likes, fmt.Sprintf(`rel_path LIKE $%d ESCAPE '\'`, argIdx+len(args)))
		args = append(args, escapeLikePrefix(p)+"%")
	}
	if len(likes) == 0 {
		return "", nil
	}
	return fmt.Sprintf(`AND doc_id NOT IN (SELECT id FROM knowledge_documents WHERE collection_id = %s AND (%s))`,
		collectionPH, strings.Join(likes, " OR ")), args
}
