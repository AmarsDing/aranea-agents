package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/ent/knowledgelinkused"
)

// ── B4 #8：wikilink 落链 recency（LinkUsageRepo 端口实现） ───────────────────

// TouchLinkUse upsert (collection, doc) 的 last_used_at；冲突键为
// (collection_id, doc_id) 唯一索引，重复落链只刷新时间戳。
func (r *knowledgeRepo) TouchLinkUse(ctx context.Context, collectionID, docID string, at time.Time) error {
	err := r.data.RW().Write(ctx).KnowledgeLinkUsed.Create().
		SetCollectionID(collectionID).
		SetDocID(docID).
		SetLastUsedAt(at.UTC()).
		OnConflictColumns(knowledgelinkused.FieldCollectionID, knowledgelinkused.FieldDocID).
		UpdateNewValues().
		Exec(ctx)
	return entErrToBizErr(err, "KNOWLEDGE")
}

// ListRecentLinkUses 按 last_used_at 降序返回 collection 内最近引用。
func (r *knowledgeRepo) ListRecentLinkUses(ctx context.Context, collectionID string, limit int) ([]bizknowledge.LinkUse, error) {
	rows, err := r.data.RW().Read(ctx).KnowledgeLinkUsed.Query().
		Where(knowledgelinkused.CollectionIDEQ(collectionID)).
		Order(knowledgelinkused.ByLastUsedAt(sql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE")
	}
	out := make([]bizknowledge.LinkUse, 0, len(rows))
	for _, row := range rows {
		out = append(out, bizknowledge.LinkUse{DocID: row.DocID, LastUsedAt: row.LastUsedAt})
	}
	return out, nil
}
