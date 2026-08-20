package data

import (
	"context"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

// 自治理知识图谱 M1-2：knowledge_access_log 检索命中日志。
// base-level 激活分（ACT-R）：B = ln( Σ_t age_days^-0.5 )，d=0.5 遗忘幂律。
// 年龄下限 1e-4 天（≈8.6s）防 0 除；同一秒内的多次命中不过度放大。

var (
	_ bizknowledge.AccessLogRepo    = (*knowledgeRepo)(nil)
	_ bizknowledge.CoActivationRepo = (*knowledgeRepo)(nil)
)

// LogAccess 批量记录检索命中（同一 query_hash 标识同批召回，Hebbian 共激活分组键）。
func (r *knowledgeRepo) LogAccess(ctx context.Context, entries []bizknowledge.AccessLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	stmt := `INSERT INTO knowledge_access_log (collection_id, doc_id, query_hash)
			 SELECT * FROM UNNEST($1::text[], $2::text[], $3::text[])`
	cols := make([]string, 0, len(entries))
	docs := make([]string, 0, len(entries))
	hashes := make([]string, 0, len(entries))
	for _, e := range entries {
		cols = append(cols, e.CollectionID)
		docs = append(docs, e.DocID)
		hashes = append(hashes, e.QueryHash)
	}
	if _, err := r.data.Postgres().ExecContext(ctx, stmt, pq.Array(cols), pq.Array(docs), pq.Array(hashes)); err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	return nil
}

// BaseLevelScores 按文档聚合 ACT-R base-level 激活分：ln(Σ age_days^-0.5)。
// 仅统计有命中史的文档（无史文档不出现在结果中，调用方按 0 处理）。
func (r *knowledgeRepo) BaseLevelScores(ctx context.Context, collectionID string, docIDs []string) (map[string]float64, error) {
	out := make(map[string]float64, len(docIDs))
	if len(docIDs) == 0 {
		return out, nil
	}
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT doc_id, LN(SUM(POWER(GREATEST(EXTRACT(EPOCH FROM (NOW() - accessed_at)) / 86400.0, 0.0001), -0.5)))
		 FROM knowledge_access_log
		 WHERE collection_id = $1 AND doc_id = ANY($2)
		 GROUP BY doc_id`,
		collectionID, pq.Array(docIDs))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	for rows.Next() {
		var docID string
		var score float64
		if err := rows.Scan(&docID, &score); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out[docID] = score
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// StrengthenCoActivations Hebbian 共激活（M1-3）：同批文档两两 upsert co_activated 边，
// weight_f += eta；无向边规范化为 doc_id<target_doc_id 单行。端点文档缺失（检索后
// 被并发删除）的对跳过，避免 FK 违例拖垮整批。
func (r *knowledgeRepo) StrengthenCoActivations(ctx context.Context, collectionID string, docIDs []string, eta float64) error {
	if len(docIDs) < 2 || eta <= 0 {
		return nil
	}
	// 先 UPDATE 命中对（含已衰减边 → valid_to 归 NULL 复活），再对未命中对 INSERT。
	// 不能单靠 ON CONFLICT：唯一索引是 partial（WHERE valid_to IS NULL），衰减边
	// 不入索引、永不触发冲突 → 会插出重复行且衰减边不被复活。
	_, err := r.data.Postgres().ExecContext(ctx, `
WITH ids AS (
	SELECT DISTINCT d FROM UNNEST($2::text[]) AS d
	JOIN knowledge_documents doc ON doc.id = d AND doc.collection_id = $1
),
pairs AS (
	SELECT DISTINCT LEAST(a.d, b.d) AS src, GREATEST(a.d, b.d) AS dst
	FROM ids a CROSS JOIN ids b
	WHERE a.d <> b.d
),
updated AS (
	UPDATE knowledge_links kl
	SET weight_f = kl.weight_f + $3, valid_to = NULL
	FROM pairs p
	WHERE kl.collection_id = $1 AND kl.doc_id = p.src AND kl.target_doc_id = p.dst
		AND kl.link_type = 'co_activated' AND kl.relation = ''
	RETURNING kl.doc_id, kl.target_doc_id
)
INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, weight_f)
SELECT $1, p.src, p.dst, 'co_activated', '', $3 FROM pairs p
WHERE NOT EXISTS (
	SELECT 1 FROM updated u WHERE u.doc_id = p.src AND u.target_doc_id = p.dst
)
ON CONFLICT (doc_id, target_doc_id, link_type, relation) WHERE valid_to IS NULL
DO UPDATE SET weight_f = knowledge_links.weight_f + EXCLUDED.weight_f, valid_to = NULL`,
		collectionID, pq.Array(docIDs), eta)
	if err != nil {
		return entErrToBizErr(err, "knowledge")
	}
	return nil
}
