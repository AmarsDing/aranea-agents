package data

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lib/pq"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// SP1-C：knowledgeBlockRepo 同时实现 ResolveIndex（解析候选/块定位）与
// UpdateDocLinkKeys（文档键 title/aliases 物化回写）。解析键与块索引同管线
// 物化（RebuildBlockIndex 内调用），故归属同一 repo；与块写分离调用，
// 失败仅解析键滞后，不影响块/refs 已提交事务。

var _ bizknowledge.ResolveIndex = (*knowledgeBlockRepo)(nil)

// ListResolveCandidates 列出可见集合内的全部候选文档（B-1：不可见集合由
// 调用方传入的 collectionIDs 裁剪，SQL 不再过滤）。title/aliases 为
// 20261204 迁移列（frontmatter 物化）；空集合 ID 列表直接短路不查库。
func (r *knowledgeBlockRepo) ListResolveCandidates(ctx context.Context, collectionIDs []string) ([]bizknowledge.ResolveDocCandidate, error) {
	if len(collectionIDs) == 0 {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT d.id, d.collection_id, d.rel_path, d.title,
		        COALESCE(d.aliases, '[]'::jsonb), c.created_at
		 FROM knowledge_documents d
		 JOIN knowledge_collections c ON c.id = d.collection_id
		 WHERE d.collection_id = ANY($1)`, pq.Array(collectionIDs))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	var out []bizknowledge.ResolveDocCandidate
	for rows.Next() {
		var (
			c          bizknowledge.ResolveDocCandidate
			aliasesRaw []byte
			createdAt  time.Time
		)
		if err := rows.Scan(&c.DocID, &c.CollectionID, &c.RelPath, &c.Title, &aliasesRaw, &createdAt); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		if len(aliasesRaw) > 0 {
			var aliases []string
			if err := json.Unmarshal(aliasesRaw, &aliases); err == nil {
				c.Aliases = aliases
			}
		}
		c.CollectionCreatedAt = createdAt
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}

// FindBlockByAnchor 按显式锚点定位块（锚点库级唯一由部分唯一索引保证，
// 此处按 doc 收敛查询）。未命中 ok=false（块级 dangling，非错误）。
func (r *knowledgeBlockRepo) FindBlockByAnchor(ctx context.Context, docID, anchor string) (string, bool, error) {
	return r.queryOneBlockID(ctx,
		`SELECT id FROM knowledge_blocks WHERE doc_id = $1 AND anchor = $2 LIMIT 1`,
		docID, anchor)
}

// FindBlockByHeadingPath 按标题路径定位 heading 块；重复标题取 ordinal 最小者
// （与 Resolver 内存自引用「取首」确定性口径一致）。未命中 ok=false。
func (r *knowledgeBlockRepo) FindBlockByHeadingPath(ctx context.Context, docID string, path []string) (string, bool, error) {
	return r.queryOneBlockID(ctx,
		`SELECT id FROM knowledge_blocks
		 WHERE doc_id = $1 AND kind = 'heading' AND heading_path = $2
		 ORDER BY ordinal LIMIT 1`,
		docID, pq.Array(path))
}

// queryOneBlockID 单行块 ID 查询（execer 无 QueryRowContext，用 rows 收敛）。
func (r *knowledgeBlockRepo) queryOneBlockID(ctx context.Context, query string, args ...any) (string, bool, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return "", false, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", false, entErrToBizErr(err, "knowledge")
		}
		return "", false, nil
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		return "", false, entErrToBizErr(err, "knowledge")
	}
	return id, true, nil
}

// UpdateDocLinkKeys 物化文档解析键（frontmatter title/aliases → documents 列）。
// 空值照常写（frontmatter 移除后键同步清除）；aliases nil 写 NULL。
func (r *knowledgeBlockRepo) UpdateDocLinkKeys(ctx context.Context, docID, title string, aliases []string) error {
	var aliasesJSON any
	if aliases != nil {
		raw, err := json.Marshal(aliases)
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		aliasesJSON = string(raw)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE knowledge_documents SET title = $2, aliases = $3 WHERE id = $1`,
		docID, title, aliasesJSON)
	return entErrToBizErr(err, "knowledge")
}
