package data

// 自治理知识图谱 M2 语义关系层持久化：semantic typed edges / 谓词词表 /
// 热文档检出 / 抽取幂等状态 / 只读实体解析。
// 派生索引纪律：semantic 边随文档增删级联、可全量重抽重建，无业务表耦合。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

var (
	_ bizknowledge.SemanticLinkRepo  = (*knowledgeRepo)(nil)
	_ bizknowledge.RelationVocabRepo = (*knowledgeRepo)(nil)
	_ bizknowledge.HotDocumentLister = (*knowledgeRepo)(nil)
	_ bizknowledge.RelationStateRepo = (*knowledgeRepo)(nil)
	_ bizknowledge.EntityIDResolver  = (*knowledgeRepo)(nil)
)

// ReplaceSemanticLinks 事务性替换某文档的全部 active semantic 出链：
// 未变化边原位刷新，消失/关闭边写 valid_to，新边插入新版本。
// relation 参与唯一键（同对文档多谓词共存）；weight_f/confidence 承载抽取置信度；
// Closed 边写入即关闭（valid_to=now，低置信留痕不进主图谱）。
func (r *knowledgeRepo) ReplaceSemanticLinks(ctx context.Context, collectionID, docID string, links []bizknowledge.SemanticLink) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		type semanticKey struct {
			target   string
			relation string
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT id, target_doc_id, relation
			 FROM knowledge_links
			 WHERE doc_id = $1 AND link_type = $2 AND valid_to IS NULL
			 FOR UPDATE`,
			docID, bizknowledge.LinkTypeSemantic)
		if err != nil {
			return err
		}
		active := make(map[semanticKey]int64)
		for rows.Next() {
			var id int64
			var key semanticKey
			if err := rows.Scan(&id, &key.target, &key.relation); err != nil {
				rows.Close()
				return err
			}
			active[key] = id
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, l := range links {
			if l.TargetDocID == "" || l.TargetDocID == docID || strings.TrimSpace(l.Relation) == "" {
				continue
			}
			confidence := l.Confidence
			if confidence <= 0 {
				confidence = 0.5
			}
			key := semanticKey{target: l.TargetDocID, relation: l.Relation}
			if id, ok := active[key]; ok {
				var updateErr error
				if l.Closed {
					_, updateErr = tx.ExecContext(ctx,
						`UPDATE knowledge_links
						 SET collection_id = $2, context = $3, weight_f = $4,
						     confidence = $4, valid_to = NOW()
						 WHERE id = $1`,
						id, collectionID, l.Context, confidence)
				} else {
					_, updateErr = tx.ExecContext(ctx,
						`UPDATE knowledge_links
						 SET collection_id = $2, context = $3, weight_f = $4,
						     confidence = $4
						 WHERE id = $1`,
						id, collectionID, l.Context, confidence)
				}
				if updateErr != nil {
					return updateErr
				}
				delete(active, key)
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO knowledge_links
					(collection_id, doc_id, target_doc_id, link_type, relation, context, weight, weight_f, confidence, valid_to)
				 VALUES ($1,$2,$3,$4,$5,$6,1,$7,$8, CASE WHEN $9 THEN NOW() ELSE NULL END)
				 ON CONFLICT (doc_id, target_doc_id, link_type, relation) WHERE valid_to IS NULL
				 DO UPDATE SET context = EXCLUDED.context, weight_f = EXCLUDED.weight_f,
				               confidence = EXCLUDED.confidence, valid_to = EXCLUDED.valid_to`,
				collectionID, docID, l.TargetDocID, bizknowledge.LinkTypeSemantic,
				l.Relation, l.Context, confidence, confidence, l.Closed); err != nil {
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

// UpsertCandidate 词表外谓词落 candidate 层：新词插入 tier=candidate；既有 candidate
// use_count+1（治理提升依据）；core/promoted 不降级不计数（WHERE 守卫）。
func (r *knowledgeRepo) UpsertCandidate(ctx context.Context, relation, proposedBy string) error {
	relation = strings.TrimSpace(relation)
	if relation == "" {
		return nil
	}
	if strings.TrimSpace(proposedBy) == "" {
		proposedBy = "llm"
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_relation_vocab (relation, tier, proposed_by)
		 VALUES ($1, 'candidate', $2)
		 ON CONFLICT (relation) DO UPDATE SET use_count = knowledge_relation_vocab.use_count + 1
		 WHERE knowledge_relation_vocab.tier = 'candidate'`,
		relation, proposedBy)
	if err != nil {
		return fmt.Errorf("upsert relation candidate: %w", err)
	}
	return nil
}

// ListHotDocuments 返回 sinceDays 窗口内命中次数 >= minHits 的文档（命中数降序，limit 截断）。
// M2 成本闸门：关系抽取只服务高价值词条。读池访问（统计源，无 read-your-writes 约束）。
func (r *knowledgeRepo) ListHotDocuments(ctx context.Context, collectionID string, sinceDays, minHits, limit int) ([]string, error) {
	if sinceDays <= 0 {
		sinceDays = 30
	}
	if minHits <= 0 {
		minHits = 1
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT doc_id FROM knowledge_access_log
		 WHERE collection_id = $1 AND accessed_at > NOW() - make_interval(days => $2)
		 GROUP BY doc_id
		 HAVING COUNT(*) >= $3
		 ORDER BY COUNT(*) DESC
		 LIMIT $4`,
		collectionID, sinceDays, minHits, limit)
	if err != nil {
		return nil, fmt.Errorf("list hot documents: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetRelationState 取文档抽取状态；未抽取过返回 found=false（非错误）。
func (r *knowledgeRepo) GetRelationState(ctx context.Context, docID string) (bizknowledge.RelationState, bool, error) {
	var st bizknowledge.RelationState
	var entAt, relAt *time.Time
	err := r.data.PostgresRead().QueryRowContext(ctx,
		`SELECT doc_id, collection_id, content_hash, entities_extracted_at, relations_extracted_at
		 FROM knowledge_relation_state WHERE doc_id = $1`, docID).
		Scan(&st.DocID, &st.CollectionID, &st.ContentHash, &entAt, &relAt)
	if errors.Is(err, sql.ErrNoRows) {
		return bizknowledge.RelationState{}, false, nil
	}
	if err != nil {
		return bizknowledge.RelationState{}, false, fmt.Errorf("get relation state: %w", err)
	}
	if entAt != nil {
		st.EntitiesExtractedAt = *entAt
	}
	if relAt != nil {
		st.RelationsExtractedAt = *relAt
	}
	return st, true, nil
}

// UpsertRelationState 登记/刷新抽取状态（按 doc_id 冲突更新；零值时间不动既有列）。
func (r *knowledgeRepo) UpsertRelationState(ctx context.Context, st bizknowledge.RelationState) error {
	if st.DocID == "" {
		return nil
	}
	var entAt, relAt *time.Time
	if !st.EntitiesExtractedAt.IsZero() {
		entAt = &st.EntitiesExtractedAt
	}
	if !st.RelationsExtractedAt.IsZero() {
		relAt = &st.RelationsExtractedAt
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_relation_state (doc_id, collection_id, content_hash, entities_extracted_at, relations_extracted_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (doc_id) DO UPDATE SET
		   collection_id = EXCLUDED.collection_id,
		   content_hash = EXCLUDED.content_hash,
		   entities_extracted_at = COALESCE(EXCLUDED.entities_extracted_at, knowledge_relation_state.entities_extracted_at),
		   relations_extracted_at = COALESCE(EXCLUDED.relations_extracted_at, knowledge_relation_state.relations_extracted_at)`,
		st.DocID, st.CollectionID, st.ContentHash, entAt, relAt)
	if err != nil {
		return fmt.Errorf("upsert relation state: %w", err)
	}
	return nil
}

// ResolveEntityIDs 批量只读解析实体名 → 实体 ID：归一化 name_norm 精确命中 →
// 别名命中 keeper；未知名缺席（不新建字典条目，与 ReplaceDocEntities 写式解析互补）。
func (r *knowledgeRepo) ResolveEntityIDs(ctx context.Context, collectionID string, names []string) (map[string]int64, error) {
	out := make(map[string]int64, len(names))
	if len(names) == 0 {
		return out, nil
	}
	// 原名 → norm（大小写/全半角变体归一）；norms 去重后批量查库。
	nameNorms := make(map[string]string, len(names))
	normSeen := make(map[string]struct{}, len(names))
	norms := make([]string, 0, len(names))
	for _, n := range names {
		norm := bizknowledge.NormalizeEntityName(n)
		if norm == "" {
			continue
		}
		nameNorms[n] = norm
		if _, ok := normSeen[norm]; !ok {
			normSeen[norm] = struct{}{}
			norms = append(norms, norm)
		}
	}
	if len(norms) == 0 {
		return out, nil
	}
	normToID := make(map[string]int64, len(norms))
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT name_norm, id FROM knowledge_entities
		 WHERE collection_id = $1 AND name_norm = ANY($2)`, collectionID, pq.Array(norms))
	if err != nil {
		return nil, fmt.Errorf("resolve entities: %w", err)
	}
	if err := func() error {
		defer rows.Close()
		for rows.Next() {
			var norm string
			var id int64
			if err := rows.Scan(&norm, &id); err != nil {
				return err
			}
			normToID[norm] = id
		}
		return rows.Err()
	}(); err != nil {
		return nil, err
	}
	// 未精确命中的走别名表（B12：合并效果跨同步持久）。
	var miss []string
	for _, norm := range norms {
		if _, ok := normToID[norm]; !ok {
			miss = append(miss, norm)
		}
	}
	if len(miss) > 0 {
		aliasRows, err := r.data.PostgresRead().QueryContext(ctx,
			`SELECT alias_norm, entity_id FROM knowledge_entity_aliases
			 WHERE collection_id = $1 AND alias_norm = ANY($2)`, collectionID, pq.Array(miss))
		if err != nil {
			return nil, fmt.Errorf("resolve entity aliases: %w", err)
		}
		if err := func() error {
			defer aliasRows.Close()
			for aliasRows.Next() {
				var norm string
				var id int64
				if err := aliasRows.Scan(&norm, &id); err != nil {
					return err
				}
				normToID[norm] = id
			}
			return aliasRows.Err()
		}(); err != nil {
			return nil, err
		}
	}
	for name, norm := range nameNorms {
		if id, ok := normToID[norm]; ok {
			out[name] = id
		}
	}
	return out, nil
}
