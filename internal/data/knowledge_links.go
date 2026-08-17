package data

// knowledge_links / knowledge_entities / knowledge_doc_entities 持久化（P2-4 双轨关联）。
// 派生索引纪律：随文档增删级联（ON DELETE CASCADE），可全量重扫重建，无业务表耦合。

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

var (
	_ bizknowledge.LinkRepo             = (*knowledgeRepo)(nil)
	_ bizknowledge.EntityRepo           = (*knowledgeRepo)(nil)
	_ bizknowledge.CollectionLinkReader = (*knowledgeRepo)(nil)
	_ bizknowledge.ActiveLinkReader     = (*knowledgeRepo)(nil)
)

// ListActiveLinks 批量读取 docIDs 一端触及的全部 active 边（valid_to IS NULL），
// 扩散激活（M1-4）逐跳 BFS 数据源。读池访问（派生索引，无 read-your-writes 约束）。
func (r *knowledgeRepo) ListActiveLinks(ctx context.Context, collectionID string, docIDs []string) ([]bizknowledge.ActiveLink, error) {
	if len(docIDs) == 0 {
		return nil, nil
	}
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT doc_id, target_doc_id, link_type, relation, weight_f
		 FROM knowledge_links
		 WHERE collection_id = $1 AND valid_to IS NULL
		   AND (doc_id = ANY($2) OR target_doc_id = ANY($2))`,
		collectionID, pq.Array(docIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bizknowledge.ActiveLink
	for rows.Next() {
		var l bizknowledge.ActiveLink
		if err := rows.Scan(&l.DocID, &l.TargetDocID, &l.LinkType, &l.Relation, &l.WeightF); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceLinks 事务性替换某文档某类型的 active 出链。未变化边原位更新，
// 消失边关闭 valid_to，新边插入新版本；历史行不物理删除。
func (r *knowledgeRepo) ReplaceLinks(ctx context.Context, collectionID, docID, linkType string, links []bizknowledge.Link) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, target_doc_id
			 FROM knowledge_links
			 WHERE doc_id = $1 AND link_type = $2 AND relation = '' AND valid_to IS NULL
			 FOR UPDATE`,
			docID, linkType)
		if err != nil {
			return err
		}
		active := make(map[string]int64)
		for rows.Next() {
			var id int64
			var target string
			if err := rows.Scan(&id, &target); err != nil {
				rows.Close()
				return err
			}
			active[target] = id
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, l := range links {
			if l.TargetDocID == "" || l.TargetDocID == docID {
				continue
			}
			weight := l.Weight
			if weight <= 0 {
				weight = 1
			}
			if id, ok := active[l.TargetDocID]; ok {
				if _, err := tx.ExecContext(ctx,
					`UPDATE knowledge_links
					 SET collection_id = $2, context = $3, weight = $4,
					     weight_f = $4::double precision, confidence = 1.0
					 WHERE id = $1`,
					id, collectionID, l.Context, weight); err != nil {
					return err
				}
				delete(active, l.TargetDocID)
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, context, weight)
				 VALUES ($1,$2,$3,$4,$5,$6)
				 ON CONFLICT (doc_id, target_doc_id, link_type, relation) WHERE valid_to IS NULL
				 DO UPDATE SET weight = EXCLUDED.weight, context = EXCLUDED.context`,
				collectionID, docID, l.TargetDocID, linkType, l.Context, weight); err != nil {
				return err
			}
		}
		if len(active) > 0 {
			ids := make([]int64, 0, len(active))
			for _, id := range active {
				ids = append(ids, id)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE knowledge_links SET valid_to = NOW()
				 WHERE id = ANY($1) AND valid_to IS NULL`,
				pq.Array(ids)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListLinks 列出文档的全部关联（出向 + 入向）；linkType 空 = 全部类型。
func (r *knowledgeRepo) ListLinks(ctx context.Context, collectionID, docID, linkType string) ([]bizknowledge.Link, error) {
	q := `SELECT id, collection_id, doc_id, target_doc_id, link_type, context, weight
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
		if err := rows.Scan(&l.ID, &l.CollectionID, &l.DocID, &l.TargetDocID, &l.LinkType, &l.Context, &l.Weight); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListCollectionLinks 列出库内全部关联（G4-B8 图谱数据源）；linkTypes 空 = 全部类型。
// 按 id 有序保证返回稳定；路径前缀过滤在 biz 层按文档集裁剪（端点须在范围内）。
func (r *knowledgeRepo) ListCollectionLinks(ctx context.Context, collectionID string, linkTypes []string) ([]bizknowledge.Link, error) {
	q := `SELECT id, collection_id, doc_id, target_doc_id, link_type, context, weight
		FROM knowledge_links
		WHERE collection_id = $1`
	args := []any{collectionID}
	if len(linkTypes) > 0 {
		q += ` AND link_type = ANY($2)`
		args = append(args, pq.Array(linkTypes))
	}
	q += ` ORDER BY id`
	rows, err := r.data.PostgresRead().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bizknowledge.Link
	for rows.Next() {
		var l bizknowledge.Link
		if err := rows.Scan(&l.ID, &l.CollectionID, &l.DocID, &l.TargetDocID, &l.LinkType, &l.Context, &l.Weight); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceDocEntities 事务性替换文档实体（G5-F B9/B12）：归一化 name_norm 查/建
// 字典条目（name 保留首见写法作展示名）→ 别名命中 keeper → 新建；同批归一化
// 撞车 mentions 求和；提及表重建 + 孤儿实体清理。返回解析后的实体 ID
// （去重、保首现序），供共现查询按 entity_id 关联。
func (r *knowledgeRepo) ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []bizknowledge.DocEntity) ([]int64, error) {
	var ids []int64
	err := r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_doc_entities WHERE doc_id = $1`, docID); err != nil {
			return err
		}
		mentionsByID := map[int64]int{}
		for _, e := range entities {
			name := strings.TrimSpace(e.Name)
			if name == "" {
				continue
			}
			norm := bizknowledge.NormalizeEntityName(name)
			if norm == "" {
				continue // 无治理价值名跳过（与迁移垃圾行清理同规）
			}
			id, err := resolveKnowledgeEntityID(ctx, tx, collectionID, name, norm, e.EntityType)
			if err != nil {
				return err
			}
			mentions := e.Mentions
			if mentions < 1 {
				mentions = 1
			}
			if _, dup := mentionsByID[id]; dup {
				mentionsByID[id] += mentions // 同批归一化撞车：mentions 求和
				continue
			}
			mentionsByID[id] = mentions
			ids = append(ids, id)
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions)
				 VALUES ($1,$2,$3,$4)`,
				collectionID, docID, id, mentionsByID[id]); err != nil {
				return err
			}
		}
		// 孤儿实体清理：不再被任何文档引用的实体删除（派生索引无残留；别名随行级联）。
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_entities e
			 WHERE e.collection_id = $1
			   AND NOT EXISTS (SELECT 1 FROM knowledge_doc_entities de WHERE de.entity_id = e.id)`,
			collectionID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// resolveKnowledgeEntityID 实体解析管线（G5-F B9/B12）：精确 name_norm → 别名命中
// keeper（合并效果跨同步持久）→ 新建条目（并发同 norm 撞唯一约束回读既有行）。
// 命中时以非空 entity_type 刷新类型；新建时 name 保留首见写法作展示名。
func resolveKnowledgeEntityID(ctx context.Context, tx *sql.Tx, collectionID, displayName, nameNorm, entityType string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM knowledge_entities WHERE collection_id = $1 AND name_norm = $2`,
		collectionID, nameNorm).Scan(&id)
	if err == nil {
		return id, refreshKnowledgeEntityType(ctx, tx, id, entityType)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx,
		`SELECT entity_id FROM knowledge_entity_aliases WHERE collection_id = $1 AND alias_norm = $2`,
		collectionID, nameNorm).Scan(&id)
	if err == nil {
		return id, refreshKnowledgeEntityType(ctx, tx, id, entityType)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO knowledge_entities (collection_id, name, entity_type, name_norm)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (collection_id, name_norm) DO NOTHING
		 RETURNING id`,
		collectionID, displayName, entityType, nameNorm).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// 并发新建撞唯一约束：回读既有行。
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM knowledge_entities WHERE collection_id = $1 AND name_norm = $2`,
			collectionID, nameNorm).Scan(&id)
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// refreshKnowledgeEntityType 命中既有实体时以非空类型刷新（空类型不清除既有值）。
func refreshKnowledgeEntityType(ctx context.Context, tx *sql.Tx, id int64, entityType string) error {
	if entityType == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE knowledge_entities SET entity_type = $2 WHERE id = $1 AND entity_type <> $2`,
		id, entityType)
	return err
}

// FindEntityCooccurrences 按实体 ID 找共享实体的其他文档（excludeDocID 除外）；
// SharedEntities 返回实体展示名（keeper 首见写法），供链接 context 标注来源。
// R-3 频次过滤：出现在超过 maxDocFreq 个文档的实体视为噪声跳过；maxDocFreq<=0 不过滤。
func (r *knowledgeRepo) FindEntityCooccurrences(ctx context.Context, collectionID string, entityIDs []int64, excludeDocID string, maxDocFreq int) ([]bizknowledge.EntityCooccurrence, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	// freq CTE 仅聚合查询实体（join 后只有 ANY($2) 的 entity_id 被消费，过滤下推语义等价，
	// 避免每次同步全 collection 聚合）；读池访问（读写分离，无 read-your-writes 约束）。
	rows, err := r.data.PostgresRead().QueryContext(ctx, `
WITH freq AS (
	SELECT entity_id, COUNT(DISTINCT doc_id) AS doc_freq
	FROM knowledge_doc_entities
	WHERE collection_id = $1 AND entity_id = ANY($2)
	GROUP BY entity_id
)
SELECT de.doc_id, e.name
FROM knowledge_doc_entities de
JOIN knowledge_entities e ON e.id = de.entity_id
JOIN freq f ON f.entity_id = de.entity_id
WHERE de.collection_id = $1
  AND de.entity_id = ANY($2)
  AND de.doc_id <> $4
  AND ($3 <= 0 OR f.doc_freq <= $3)
ORDER BY de.doc_id, e.name`,
		collectionID, pq.Array(entityIDs), maxDocFreq, excludeDocID)
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
