package data

// G-33 图谱卡片（ADR-KN-20261020-H）：knowledge_graph_embeddings 仓储——
// 写侧对账投影工人六端口 + 读侧检索三窄口。Raw SQL + DDL Registry 双轨
// （域内先例：entities/links/blocks 不入 ent）。可见性红线：卡片表自身无
// visibility 列，所有读路径一律经宿主文档投影下推 G-08 中央谓词
// （visibleDocIDsSubquery——同一中央扩点，禁手写可见性分支）。

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

var (
	_ bizknowledge.GraphCardStore           = (*knowledgeRepo)(nil)
	_ bizknowledge.GraphCardVectorSearcher  = (*knowledgeRepo)(nil)
	_ bizknowledge.GraphCardLexicalSearcher = (*knowledgeRepo)(nil)
	_ bizknowledge.GraphCardHostDocLister   = (*knowledgeRepo)(nil)
)

// ── 写侧（ProjectionWorker 六端口） ─────────────────────────────────────────

// ListGraphCardCollections 语义层集合（embedding_model<>”——词法库不物化
// 卡片行，工人空转闸在 SQL 层即落定）。
func (r *knowledgeRepo) ListGraphCardCollections(ctx context.Context) ([]bizknowledge.GraphProjectionCollection, error) {
	rows, err := r.data.Postgres().QueryContext(ctx,
		`SELECT id FROM knowledge_collections WHERE COALESCE(NULLIF(embedding_model, ''), '') <> '' ORDER BY id`)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.GraphProjectionCollection
	for rows.Next() {
		var c bizknowledge.GraphProjectionCollection
		if err := rows.Scan(&c.ID); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, c)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

func (r *knowledgeRepo) ListGraphCardEntitySources(ctx context.Context, collectionID string) ([]bizknowledge.GraphCardEntitySource, error) {
	rows, err := r.data.Postgres().QueryContext(ctx,
		`SELECT e.id, e.name, COALESCE(e.entity_type, ''),
		        COALESCE(array_agg(a.alias_norm ORDER BY a.alias_norm)
		                 FILTER (WHERE a.alias_norm IS NOT NULL), '{}')
		 FROM knowledge_entities e
		 LEFT JOIN knowledge_entity_aliases a ON a.entity_id = e.id
		 WHERE e.collection_id = $1
		 GROUP BY e.id, e.name, e.entity_type
		 ORDER BY e.id`, collectionID)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.GraphCardEntitySource
	for rows.Next() {
		var s bizknowledge.GraphCardEntitySource
		if err := rows.Scan(&s.EntityID, &s.Name, &s.EntityType, pq.Array(&s.Aliases)); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// ListGraphCardEdgeSources active semantic 边（ADR-G 门槛：link_type='semantic'
// AND valid_to IS NULL）+ 目标文档题面（宾语展示名出处）。
func (r *knowledgeRepo) ListGraphCardEdgeSources(ctx context.Context, collectionID string) ([]bizknowledge.GraphCardEdgeSource, error) {
	rows, err := r.data.Postgres().QueryContext(ctx,
		`SELECT l.id, l.relation, COALESCE(l.context, ''),
		        COALESCE(NULLIF(d.title, ''), ''), COALESCE(d.rel_path, '')
		 FROM knowledge_links l
		 JOIN knowledge_documents d ON d.id = l.target_doc_id
		 WHERE l.collection_id = $1 AND l.link_type = $2 AND l.valid_to IS NULL
		 ORDER BY l.id`, collectionID, bizknowledge.LinkTypeSemantic)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.GraphCardEdgeSource
	for rows.Next() {
		var s bizknowledge.GraphCardEdgeSource
		if err := rows.Scan(&s.LinkID, &s.Relation, &s.Context, &s.TargetTitle, &s.TargetRelPath); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

func (r *knowledgeRepo) ListGraphCards(ctx context.Context, collectionID string) ([]bizknowledge.GraphCardRow, error) {
	// G-39：SELECT 带 dim——对账（PlanGraphCardReconcile）据此识别换模型后的
	// 维度漂移并召回重嵌；纯 hash 对账会让旧 dim 卡永久退出向量腿。
	rows, err := r.data.Postgres().QueryContext(ctx,
		`SELECT kind, ref_id, collection_id, text, source_hash, dim
		 FROM knowledge_graph_embeddings WHERE collection_id = $1 ORDER BY kind, ref_id`, collectionID)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.GraphCardRow
	for rows.Next() {
		var c bizknowledge.GraphCardRow
		if err := rows.Scan(&c.Kind, &c.RefID, &c.CollectionID, &c.Text, &c.SourceHash, &c.Dim); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, c)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// UpsertGraphCards (kind,ref_id) 幂等重嵌（缺卡/漂移共用一径）。
// G-39（ADR-KN-20261021-G）：dim 列跟随 len(embedding) 落账（多代模型卡片
// 共存，embedding 裸型化后同表多 dim 合法），per-dim partial HNSW 经
// ensureChunkEmbeddingDimIndexes 注册。空向量 upsert 保向量/dim 不覆盖
// （Embedder 批内空切片不得抹掉既有向量使卡片静默退出向量腿）——文本/
// hash 照常推进，缺维由 PlanGraphCardReconcile 的 dim 口径召回重嵌。
func (r *knowledgeRepo) UpsertGraphCards(ctx context.Context, cards []bizknowledge.GraphCardUpsert) error {
	if len(cards) == 0 {
		return nil
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO knowledge_graph_embeddings (kind, ref_id, collection_id, text, source_hash, dim, embedding, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			 ON CONFLICT (kind, ref_id) DO UPDATE SET
			   collection_id = EXCLUDED.collection_id,
			   text = EXCLUDED.text,
			   source_hash = EXCLUDED.source_hash,
			   dim = CASE WHEN EXCLUDED.dim > 0 THEN EXCLUDED.dim ELSE knowledge_graph_embeddings.dim END,
			   embedding = CASE WHEN EXCLUDED.embedding IS NOT NULL THEN EXCLUDED.embedding ELSE knowledge_graph_embeddings.embedding END,
			   updated_at = NOW()`)
		if err != nil {
			return entErrToBizErr(err, "knowledge")
		}
		defer stmt.Close()
		for _, c := range cards {
			var vec interface{}
			if len(c.Embedding) > 0 {
				// pgvector.NewVector(nil) 会生成非法 '[]'（同 knowledge_chunks 纪律）。
				vec = pgvector.NewVector(c.Embedding)
			}
			if _, err := stmt.ExecContext(ctx, c.Kind, c.RefID, c.CollectionID, c.Text, c.SourceHash, len(c.Embedding), vec); err != nil {
				return entErrToBizErr(err, "knowledge")
			}
		}
		return nil
	})
}

// DeleteGraphCards 墓碑删除（实体归并硬删/边闭合的孤儿行）。
func (r *knowledgeRepo) DeleteGraphCards(ctx context.Context, keys []bizknowledge.GraphCardKey) error {
	if len(keys) == 0 {
		return nil
	}
	kinds := make([]string, len(keys))
	ids := make([]int64, len(keys))
	for i, k := range keys {
		kinds[i] = k.Kind
		ids[i] = k.RefID
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`DELETE FROM knowledge_graph_embeddings
		 WHERE (kind, ref_id) IN (SELECT * FROM unnest($1::text[], $2::bigint[]))`,
		pq.Array(kinds), pq.Array(ids))
	return entErrToBizErr(err, "knowledge")
}

// ── 读侧（检索三窄口） ─────────────────────────────────────────────────────

// SearchGraphCards 卡片 ANN（kind 过滤）：entity 卡至少一个宿主文档可见、
// edge 卡双端各至少一条可见——宿主投影 EXISTS 下推同一中央谓词。
func (r *knowledgeRepo) SearchGraphCards(ctx context.Context, collectionID, kind string, vec []float32, limit int) ([]bizknowledge.GraphCardHit, error) {
	if len(vec) == 0 || limit <= 0 {
		return nil, nil
	}
	args := []any{kind, collectionID, pgvector.NewVector(vec)}
	vis, visArgs := visibleDocIDsSubquery(ctx, "$2", len(args)+1)
	var whereVis string
	switch kind {
	case bizknowledge.GraphCardKindEdge:
		if vis != "" {
			// 双端点纪律（docBothEndpointsVisibleClause 同形）：同一 $n 双处复用。
			whereVis = fmt.Sprintf("\n  AND l.doc_id IN (%s) AND l.target_doc_id IN (%s)", vis, vis)
		}
	default:
		kind = bizknowledge.GraphCardKindEntity
		if vis != "" {
			whereVis = fmt.Sprintf(`
  AND EXISTS (
    SELECT 1 FROM knowledge_doc_entities de
    WHERE de.entity_id = t.ref_id AND de.collection_id = t.collection_id
      AND de.doc_id IN (%s)
  )`, vis)
		}
	}
	args = append(args, visArgs...)
	limitPH := len(args) + 1
	args = append(args, limit)

	join := ""
	if kind == bizknowledge.GraphCardKindEdge {
		join = "JOIN knowledge_links l ON l.id = t.ref_id"
	}
	// G-39（ADR-KN-20261021-G）：dim 按查询向量长度字面值内联（注入安全：
	// 纯 int；generic plan 下 per-dim partial HNSW 谓词可证），跨 dim 卡片
	// 不进比较桶——multi-dim 共存后 22P02 结构性不可能。ORDER BY 用 cast
	// 表达式 `t.embedding::vector(N)`：索引建在表达式上（裸型列 pgvector
	// 拒绝建 opclass，22023），查询侧必须同形方能命中。
	raw := fmt.Sprintf(`
SELECT t.kind, t.ref_id, t.text, (1 - (t.embedding <=> $3::vector)) AS score
FROM knowledge_graph_embeddings t %s
WHERE t.kind = $1 AND t.collection_id = $2 AND t.embedding IS NOT NULL AND t.dim = %d%s
ORDER BY t.embedding::vector(%d) <=> $3::vector
LIMIT $%d`, join, len(vec), whereVis, len(vec), limitPH)

	tx, err := r.data.Postgres().BeginTx(ctx, nil)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(hnswEfSearch())); err != nil {
		r.lg.Warn("knowledge hnsw.ef_search unavailable",
			loggateway.StepID("data.knowledge.hnsw_ef_search"),
			loggateway.Err(err))
	}
	rows, err := tx.QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	hits, err := scanGraphCardHits(rows)
	if err != nil {
		return nil, err
	}
	return hits, entErrToBizErr(tx.Commit(), "knowledge")
}

// SearchEntityCardsLexical 词法降级：实体名 name_norm 精确/前缀 ∪ alias_norm
// 精确命中（词法库不物化卡片行——卡文现场按 ADR-G 同一构造器拼装，
// edge 腿此模式静默关闭）。命中分仅作腿内排序/EntityScore 分解语义记录：
// exact=1.0、alias exact=0.9、prefix=0.5。
func (r *knowledgeRepo) SearchEntityCardsLexical(ctx context.Context, collectionID, query string, limit int) ([]bizknowledge.GraphCardHit, error) {
	q := bizknowledge.NormalizeEntityName(query)
	if q == "" || limit <= 0 {
		return nil, nil
	}
	args := []any{collectionID, q}
	vis, visArgs := visibleDocIDsSubquery(ctx, "$1", len(args)+1)
	whereVis := ""
	if vis != "" {
		whereVis = fmt.Sprintf(`
  AND EXISTS (
    SELECT 1 FROM knowledge_doc_entities de
    WHERE de.entity_id = e.id AND de.collection_id = e.collection_id
      AND de.doc_id IN (%s)
  )`, vis)
	}
	args = append(args, visArgs...)
	// 前缀 LIKE 通配符转义（防御路径纪律，同 monitor_trace_query.go:97——
	// v1 装配下词法腿仅 searchRRF 喂向量后可达，但独立调用面不假设上游过滤）。
	likePH := len(args) + 1
	args = append(args, escapeLikePattern(q)+"%")
	limitPH := len(args) + 1
	args = append(args, limit)
	raw := fmt.Sprintf(`
SELECT e.id, e.name, COALESCE(e.entity_type, ''),
       CASE WHEN e.name_norm = $2 THEN 1.0
            WHEN EXISTS (SELECT 1 FROM knowledge_entity_aliases a
                         WHERE a.entity_id = e.id AND a.alias_norm = $2) THEN 0.9
            ELSE 0.5 END AS score
FROM knowledge_entities e
WHERE e.collection_id = $1
  AND (e.name_norm = $2 OR e.name_norm LIKE $%d ESCAPE '\'
       OR EXISTS (SELECT 1 FROM knowledge_entity_aliases a
                  WHERE a.entity_id = e.id AND a.alias_norm = $2))%s
ORDER BY score DESC, e.name ASC
LIMIT $%d`, likePH, whereVis, limitPH)

	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	type entityRow struct {
		id    int64
		name  string
		etype string
		score float32
	}
	var found []entityRow
	for rows.Next() {
		var row entityRow
		if err := rows.Scan(&row.id, &row.name, &row.etype, &row.score); err != nil {
			rows.Close()
			return nil, entErrToBizErr(err, "knowledge")
		}
		found = append(found, row)
	}
	if err := rows.Close(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	if len(found) == 0 {
		return nil, nil
	}
	// 别名随附（卡文与向量档同一构造器，保证词法/向量两径文本同源）。
	ids := make([]int64, len(found))
	for i, f := range found {
		ids[i] = f.id
	}
	aliasRows, err := r.data.Postgres().QueryContext(ctx,
		`SELECT entity_id, alias_norm FROM knowledge_entity_aliases
		 WHERE collection_id = $1 AND entity_id = ANY($2::bigint[]) ORDER BY alias_norm`,
		collectionID, pq.Array(ids))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	aliases := make(map[int64][]string, len(found))
	for aliasRows.Next() {
		var id int64
		var norm string
		if err := aliasRows.Scan(&id, &norm); err != nil {
			aliasRows.Close()
			return nil, entErrToBizErr(err, "knowledge")
		}
		aliases[id] = append(aliases[id], norm)
	}
	if err := aliasRows.Close(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	if err := aliasRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	hits := make([]bizknowledge.GraphCardHit, 0, len(found))
	for _, f := range found {
		hits = append(hits, bizknowledge.GraphCardHit{
			Kind:  bizknowledge.GraphCardKindEntity,
			RefID: f.id,
			Text:  bizknowledge.EntityCardText(f.name, f.etype, aliases[f.id]),
			Score: f.score,
		})
	}
	return hits, nil
}

// ListGraphCardHostDocs 卡 → 宿主文档投影（G-08 谓词同一子查询扩点）：
// entity=提及该实体的可见文档（mentions 降序）；edge=双端可见文档（doc 端
// 先于 target 端，稳定序）。
func (r *knowledgeRepo) ListGraphCardHostDocs(ctx context.Context, collectionID, kind string, refID int64, limit int) ([]string, error) {
	if refID <= 0 || limit <= 0 {
		return nil, nil
	}
	args := []any{refID, collectionID}
	vis, visArgs := visibleDocIDsSubquery(ctx, "$2", len(args)+1)
	args = append(args, visArgs...)
	limitPH := len(args) + 1
	args = append(args, limit)

	var raw string
	if kind == bizknowledge.GraphCardKindEdge {
		whereVis := ""
		if vis != "" {
			whereVis = fmt.Sprintf("WHERE eps.doc_id IN (%s)", vis)
		}
		raw = fmt.Sprintf(`
SELECT eps.doc_id FROM (
  SELECT l.doc_id AS doc_id, 0 AS ord FROM knowledge_links l WHERE l.id = $1
  UNION ALL
  SELECT l.target_doc_id, 1 FROM knowledge_links l WHERE l.id = $1
) eps %s
ORDER BY eps.ord, eps.doc_id
LIMIT $%d`, whereVis, limitPH)
	} else {
		whereVis := ""
		if vis != "" {
			whereVis = fmt.Sprintf("AND de.doc_id IN (%s)", vis)
		}
		raw = fmt.Sprintf(`
SELECT de.doc_id FROM knowledge_doc_entities de
WHERE de.entity_id = $1 AND de.collection_id = $2 %s
ORDER BY de.mentions DESC, de.doc_id ASC
LIMIT $%d`, whereVis, limitPH)
	}
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, id)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

func scanGraphCardHits(rows *sql.Rows) ([]bizknowledge.GraphCardHit, error) {
	var hits []bizknowledge.GraphCardHit
	for rows.Next() {
		var h bizknowledge.GraphCardHit
		if err := rows.Scan(&h.Kind, &h.RefID, &h.Text, &h.Score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return hits, nil
}
