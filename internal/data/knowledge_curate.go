package data

// 自治理知识图谱 M4 自治理层持久化：词条治理数据端口。
// 数据源全部复用 M1（links 双时态/access_log）+ M2（relation_vocab）+ M3（提案表）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

var _ bizknowledge.KnowledgeCurateRepo = (*knowledgeRepo)(nil)

// DecayCoActivatedEdges 周期衰减：活跃 co_activated 边 weight_f *= factor，
// 跌破 minWeight 的置 valid_to 关闭（失效不删除，留痕可审计）。dryRun 只 COUNT 预估。
func (r *knowledgeRepo) DecayCoActivatedEdges(ctx context.Context, collectionID string, factor, minWeight float64, dryRun bool) (decayed, closed int, err error) {
	if dryRun {
		err = r.data.PostgresRead().QueryRowContext(ctx,
			`SELECT COUNT(*), COUNT(*) FILTER (WHERE weight_f * $2 < $3)
			 FROM knowledge_links
			 WHERE collection_id = $1 AND link_type = 'co_activated' AND valid_to IS NULL`,
			collectionID, factor, minWeight).Scan(&decayed, &closed)
		if err != nil {
			return 0, 0, fmt.Errorf("estimate decay: %w", err)
		}
		return decayed, closed, nil
	}
	res, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_links SET weight_f = weight_f * $2
		 WHERE collection_id = $1 AND link_type = 'co_activated' AND valid_to IS NULL`,
		collectionID, factor)
	if err != nil {
		return 0, 0, fmt.Errorf("decay co_activated edges: %w", err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil {
		decayed = int(n)
	}
	res, err = r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_links SET valid_to = NOW()
		 WHERE collection_id = $1 AND link_type = 'co_activated' AND valid_to IS NULL AND weight_f < $2`,
		collectionID, minWeight)
	if err != nil {
		return decayed, 0, fmt.Errorf("close decayed edges: %w", err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil {
		closed = int(n)
	}
	return decayed, closed, nil
}

// ListPromotableRelations 列出 use_count 达阈值的 candidate 谓词（受控涌现提升候选）。
func (r *knowledgeRepo) ListPromotableRelations(ctx context.Context, minUseCount int) ([]string, error) {
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT relation FROM knowledge_relation_vocab
		 WHERE tier = 'candidate' AND use_count >= $1
		 ORDER BY use_count DESC, relation`, minUseCount)
	if err != nil {
		return nil, fmt.Errorf("list promotable relations: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// PromoteRelation candidate → promoted（幂等：非 candidate 不动）。
func (r *knowledgeRepo) PromoteRelation(ctx context.Context, relation string) error {
	if strings.TrimSpace(relation) == "" {
		return nil
	}
	if _, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_relation_vocab SET tier = 'promoted'
		 WHERE relation = $1 AND tier = 'candidate'`, relation); err != nil {
		return fmt.Errorf("promote relation: %w", err)
	}
	return nil
}

// ListStaleEntries 陈旧词条候选：entries/ 下出向 semantic 边关闭比例 ≥0.5
// （曾经的语义关系大半失效）且超 inactiveDays 未被检索（含从未检索）。
func (r *knowledgeRepo) ListStaleEntries(ctx context.Context, collectionID string, inactiveDays, limit int) ([]bizknowledge.StaleEntryStat, error) {
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT d.id, d.rel_path,
		        COALESCE(EXTRACT(DAY FROM (NOW() - la.last_access))::int, 999999) AS last_access_days,
		        ls.closed_ratio
		 FROM knowledge_documents d
		 LEFT JOIN (
		   SELECT doc_id, MAX(accessed_at) AS last_access
		   FROM knowledge_access_log GROUP BY doc_id
		 ) la ON la.doc_id = d.id
		 JOIN (
		   SELECT doc_id,
		          COUNT(*) FILTER (WHERE valid_to IS NOT NULL)::float8 / NULLIF(COUNT(*), 0) AS closed_ratio
		   FROM knowledge_links WHERE link_type = 'semantic' GROUP BY doc_id
		 ) ls ON ls.doc_id = d.id
		 WHERE d.collection_id = $1 AND d.rel_path LIKE 'entries/%'
		   AND (la.last_access IS NULL OR la.last_access < NOW() - make_interval(days => $2))
		   AND ls.closed_ratio >= 0.5
		 ORDER BY last_access_days DESC
		 LIMIT $3`, collectionID, inactiveDays, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale entries: %w", err)
	}
	defer rows.Close()
	var out []bizknowledge.StaleEntryStat
	for rows.Next() {
		var s bizknowledge.StaleEntryStat
		if err := rows.Scan(&s.DocID, &s.RelPath, &s.LastAccessDays, &s.ClosedRatio); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListOrphanEntries 孤儿词条候选：entries/ 下无任何 active 边（出向+入向）
// 且超 inactiveDays 未被检索（含从未检索）。
func (r *knowledgeRepo) ListOrphanEntries(ctx context.Context, collectionID string, inactiveDays, limit int) ([]bizknowledge.OrphanEntryStat, error) {
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT d.id, d.rel_path,
		        COALESCE(EXTRACT(DAY FROM (NOW() - la.last_access))::int, 999999) AS last_access_days
		 FROM knowledge_documents d
		 LEFT JOIN (
		   SELECT doc_id, MAX(accessed_at) AS last_access
		   FROM knowledge_access_log GROUP BY doc_id
		 ) la ON la.doc_id = d.id
		 WHERE d.collection_id = $1 AND d.rel_path LIKE 'entries/%'
		   AND (la.last_access IS NULL OR la.last_access < NOW() - make_interval(days => $2))
		   AND NOT EXISTS (
		     SELECT 1 FROM knowledge_links l
		     WHERE l.valid_to IS NULL AND (l.doc_id = d.id OR l.target_doc_id = d.id)
		   )
		 ORDER BY last_access_days DESC
		 LIMIT $3`, collectionID, inactiveDays, limit)
	if err != nil {
		return nil, fmt.Errorf("list orphan entries: %w", err)
	}
	defer rows.Close()
	var out []bizknowledge.OrphanEntryStat
	for rows.Next() {
		var o bizknowledge.OrphanEntryStat
		if err := rows.Scan(&o.DocID, &o.RelPath, &o.LastAccessDays); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListContradictsEdges active contradicts 语义边（M2 抽取产物，需人工仲裁）。
func (r *knowledgeRepo) ListContradictsEdges(ctx context.Context, collectionID string, limit int) ([]bizknowledge.ContradictsEdgeStat, error) {
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT doc_id, target_doc_id, context, confidence
		 FROM knowledge_links
		 WHERE collection_id = $1 AND link_type = 'semantic' AND relation = 'contradicts' AND valid_to IS NULL
		 ORDER BY confidence DESC
		 LIMIT $2`, collectionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list contradicts edges: %w", err)
	}
	defer rows.Close()
	var out []bizknowledge.ContradictsEdgeStat
	for rows.Next() {
		var c bizknowledge.ContradictsEdgeStat
		if err := rows.Scan(&c.DocID, &c.TargetDocID, &c.Context, &c.Confidence); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasProposal 同 kind+dedup_key 且 status 在 statuses 内的提案是否已存在（周期去重）。
func (r *knowledgeRepo) HasProposal(ctx context.Context, collectionID, kind, dedupKey string, statuses []string) (bool, error) {
	if len(statuses) == 0 {
		statuses = []string{bizknowledge.ProposalStatusPending}
	}
	var exists bool
	if err := r.data.PostgresRead().QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM knowledge_governance_proposal
		   WHERE collection_id = $1 AND kind = $2
		     AND payload->>'dedup_key' = $3
		     AND status = ANY($4)
		 )`, collectionID, kind, dedupKey, pq.Array(statuses)).Scan(&exists); err != nil {
		return false, fmt.Errorf("has proposal: %w", err)
	}
	return exists, nil
}

// ResolveGovernanceProposal 人工二审闭环：pending → applied/rejected（其他状态不动）。
func (r *knowledgeRepo) ResolveGovernanceProposal(ctx context.Context, id int64, status string) error {
	res, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_governance_proposal SET status = $2, resolved_at = NOW()
		 WHERE id = $1 AND status = 'pending'`, id, status)
	if err != nil {
		return fmt.Errorf("resolve governance proposal: %w", err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil && n == 0 {
		return fmt.Errorf("proposal %d not found or not pending", id)
	}
	return nil
}

// ListGovernanceProposals 治理提案列表（人工二审出口）。collectionID/status 空 = 不过滤。
func (r *knowledgeRepo) ListGovernanceProposals(ctx context.Context, collectionID, status string, limit int) ([]bizknowledge.GovernanceProposalView, error) {
	q := `SELECT id, collection_id, kind, risk, status, payload, created_at, resolved_at
		 FROM knowledge_governance_proposal`
	var args []any
	var where []string
	if collectionID != "" {
		args = append(args, collectionID)
		where = append(where, fmt.Sprintf("collection_id = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	args = append(args, limit)
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, len(args))

	rows, err := r.data.PostgresRead().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list governance proposals: %w", err)
	}
	defer rows.Close()
	var out []bizknowledge.GovernanceProposalView
	for rows.Next() {
		var v bizknowledge.GovernanceProposalView
		var raw []byte
		var resolvedAt sql.NullTime
		if err := rows.Scan(&v.ID, &v.CollectionID, &v.Kind, &v.Risk, &v.Status, &raw, &v.CreatedAt, &resolvedAt); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			v.ResolvedAt = resolvedAt.Time
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &v.Payload) // 载荷展示用，坏 JSON 不阻断列表
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
