package data

// knowledge_links / knowledge_entities / knowledge_doc_entities 持久化（P2-4 双轨关联）。
// 派生索引纪律：随文档增删级联（ON DELETE CASCADE），可全量重扫重建，无业务表耦合。

import (
	"context"
	"database/sql"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

var (
	_ bizknowledge.LinkRepo   = (*knowledgeRepo)(nil)
	_ bizknowledge.EntityRepo = (*knowledgeRepo)(nil)
)

// ReplaceLinks 事务性替换某文档某类型的全部出链（删旧 + 插新；空切片 = 仅清理）。
// 同 (doc,target,type) 去重靠 knowledge_links_unique 唯一索引兜底（ON CONFLICT 跳过）。
func (r *knowledgeRepo) ReplaceLinks(ctx context.Context, collectionID, docID, linkType string, links []bizknowledge.Link) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_links WHERE doc_id = $1 AND link_type = $2`, docID, linkType); err != nil {
			return err
		}
		for _, l := range links {
			if l.TargetDocID == "" || l.TargetDocID == docID {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, context)
				 VALUES ($1,$2,$3,$4,$5)
				 ON CONFLICT (doc_id, target_doc_id, link_type) DO NOTHING`,
				collectionID, docID, l.TargetDocID, linkType, l.Context); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListLinks 列出文档的全部关联（出向 + 入向）；linkType 空 = 全部类型。
func (r *knowledgeRepo) ListLinks(ctx context.Context, collectionID, docID, linkType string) ([]bizknowledge.Link, error) {
	q := `SELECT id, collection_id, doc_id, target_doc_id, link_type, context
		FROM knowledge_links
		WHERE collection_id = $1 AND (doc_id = $2 OR target_doc_id = $2)`
	args := []any{collectionID, docID}
	if linkType != "" {
		q += ` AND link_type = $3`
		args = append(args, linkType)
	}
	q += ` ORDER BY id`
	rows, err := r.data.Postgres().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bizknowledge.Link
	for rows.Next() {
		var l bizknowledge.Link
		if err := rows.Scan(&l.ID, &l.CollectionID, &l.DocID, &l.TargetDocID, &l.LinkType, &l.Context); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceDocEntities 事务性替换文档实体：实体表 upsert + 提及表重建 + 孤儿实体清理。
func (r *knowledgeRepo) ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []bizknowledge.DocEntity) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_doc_entities WHERE doc_id = $1`, docID); err != nil {
			return err
		}
		for _, e := range entities {
			if e.Name == "" {
				continue
			}
			mentions := e.Mentions
			if mentions < 1 {
				mentions = 1
			}
			var entityID int64
			if err := tx.QueryRowContext(ctx,
				`INSERT INTO knowledge_entities (collection_id, name, entity_type)
				 VALUES ($1,$2,$3)
				 ON CONFLICT (collection_id, name) DO UPDATE SET entity_type = EXCLUDED.entity_type
				 RETURNING id`,
				collectionID, e.Name, e.EntityType).Scan(&entityID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions)
				 VALUES ($1,$2,$3,$4)`,
				collectionID, docID, entityID, mentions); err != nil {
				return err
			}
		}
		// 孤儿实体清理：不再被任何文档引用的实体删除（派生索引无残留）。
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_entities e
			 WHERE e.collection_id = $1
			   AND NOT EXISTS (SELECT 1 FROM knowledge_doc_entities de WHERE de.entity_id = e.id)`,
			collectionID); err != nil {
			return err
		}
		return nil
	})
}

// FindEntityCooccurrences 找共享实体的其他文档（excludeDocID 除外）。
// R-3 频次过滤：出现在超过 maxDocFreq 个文档的实体视为噪声跳过；maxDocFreq<=0 不过滤。
func (r *knowledgeRepo) FindEntityCooccurrences(ctx context.Context, collectionID string, entityNames []string, excludeDocID string, maxDocFreq int) ([]bizknowledge.EntityCooccurrence, error) {
	if len(entityNames) == 0 {
		return nil, nil
	}
	rows, err := r.data.Postgres().QueryContext(ctx, `
WITH target AS (
	SELECT id, name FROM knowledge_entities
	WHERE collection_id = $1 AND name = ANY($2)
),
freq AS (
	SELECT entity_id, COUNT(DISTINCT doc_id) AS doc_freq
	FROM knowledge_doc_entities
	WHERE collection_id = $1
	GROUP BY entity_id
)
SELECT de.doc_id, t.name
FROM knowledge_doc_entities de
JOIN target t ON t.id = de.entity_id
JOIN freq f ON f.entity_id = t.id
WHERE de.collection_id = $1
  AND de.doc_id <> $4
  AND ($3 <= 0 OR f.doc_freq <= $3)
ORDER BY de.doc_id, t.name`,
		collectionID, pq.Array(entityNames), maxDocFreq, excludeDocID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var order []string
	shared := map[string][]string{}
	for rows.Next() {
		var docID, name string
		if err := rows.Scan(&docID, &name); err != nil {
			return nil, err
		}
		if _, ok := shared[docID]; !ok {
			order = append(order, docID)
		}
		shared[docID] = append(shared[docID], name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]bizknowledge.EntityCooccurrence, 0, len(order))
	for _, docID := range order {
		out = append(out, bizknowledge.EntityCooccurrence{DocID: docID, SharedEntities: shared[docID]})
	}
	return out, nil
}
