package data

import (
	"context"
	"strconv"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
)

// ── P2-7：unlinked mentions 候选扫描（DocContentSearcher 端口实现） ──────────

// SearchDocContentMentions 返回集合内正文含 needle（ILIKE 大小写不敏感）的
// 文档全文投影，供 biz 层做 [[wikilink]] 剔除与精确计数。needle 中的 LIKE
// 通配符（%/_/\）逐字转义；按 doc id 字典序输出，至多 limit 条。
func (r *knowledgeRepo) SearchDocContentMentions(ctx context.Context, collectionID, needle, excludeDocID string, limit int) ([]bizknowledge.DocContentHit, error) {
	if limit <= 0 {
		limit = 200
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(needle)
	q := `SELECT d.id, COALESCE(NULLIF(d.rel_path, ''), d.source), d.content_text
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.collection_id = $1 AND d.id <> $2 AND d.content_text ILIKE '%' || $3 || '%' ESCAPE '\'`
	args := []any{collectionID, excludeDocID, esc}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $4 OR c.workspace = '')`
		args = append(args, ws)
	}
	q += ` ORDER BY d.id LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	out := make([]bizknowledge.DocContentHit, 0, 16)
	for rows.Next() {
		var h bizknowledge.DocContentHit
		if err := rows.Scan(&h.DocID, &h.DocName, &h.Content); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}
