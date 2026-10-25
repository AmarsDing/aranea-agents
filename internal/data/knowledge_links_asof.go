package data

// G-05-asof（ADR-KN-20261020-J）：双时态 as-of 读实现。
// 谓词代数（生产可达状态推导——非 co_activated 行 valid_from/recorded_at 零更新、
// valid_to 恰在关闭时刻 NOW() 升迁一次，故 R 轴语义精确）：
//
//	T-only → tstzrange(valid_from, COALESCE(valid_to,'infinity')) @> $T   （GiST valid_idx）
//	R-only → recorded_at <= $R AND (valid_to IS NULL OR valid_to > $R)
//	T∧R    → recorded_at <= $R AND ( @>T OR (valid_from <= $T AND valid_to > $R) )
//
// 失真钉死：任何 as-of 象限排除 co_activated（原位复活抹除衰减关闭区间）；
// closed-on-arrival 空区间（valid_from==valid_to）对全部 range 谓词天然不可见。
// 红线：zero-span 必须字节级委托原函数——current 路径的 `valid_to IS NULL`
// 原文本与 partial index（knowledge_links_active_idx）不得被间接重写波及。

import (
	"context"
	"fmt"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	"github.com/lib/pq"
)

var (
	_ bizknowledge.TemporalActiveLinkReader     = (*knowledgeRepo)(nil)
	_ bizknowledge.TemporalCollectionLinkReader = (*knowledgeRepo)(nil)
)

// asOfPredicate 生成非 zero-span 的时态谓词语句与追加参数；base 为下一个占位符序号。
// 调用方负责在 WHERE 中自行叠加 `link_type <> 'co_activated'`（与谓词并列）。
func asOfPredicate(span bizknowledge.TemporalSpan, base int) (string, []any) {
	switch {
	case span.ValidOn() && span.RecordedOn():
		t, r := fmt.Sprintf("$%d", base), fmt.Sprintf("$%d", base+1)
		return `recorded_at <= ` + r + `
	AND (tstzrange(valid_from, COALESCE(valid_to, 'infinity')) @> ` + t + `::timestamptz
	  OR (valid_from <= ` + t + ` AND valid_to > ` + r + `))`,
			[]any{span.ValidAt, span.RecordedAt}
	case span.ValidOn():
		// @> 的右操作数必须显式 ::timestamptz：pq 以未知类型发送 time.Time
		// 参数时，@> 会优先解析为 range-contains-range（参数按 tstzrange
		// 解析时间戳文本 → 22P02 malformed range literal）。
		return `tstzrange(valid_from, COALESCE(valid_to, 'infinity')) @> ` + fmt.Sprintf("$%d", base) + `::timestamptz`,
			[]any{span.ValidAt}
	default: // R-only
		r := fmt.Sprintf("$%d", base)
		return `recorded_at <= ` + r + ` AND (valid_to IS NULL OR valid_to > ` + r + `)`,
			[]any{span.RecordedAt}
	}
}

// ListActiveLinksAsOf 时态版扩散逐跳读。投影/ACL/读池与 ListActiveLinks 一致；
// span 零值字节级委托原函数（含 co_activated），非零按四象限且排除 co_activated。
func (r *knowledgeRepo) ListActiveLinksAsOf(ctx context.Context, collectionID string, docIDs []string, span bizknowledge.TemporalSpan) ([]bizknowledge.ActiveLink, error) {
	if !span.Any() {
		return r.ListActiveLinks(ctx, collectionID, docIDs)
	}
	if len(docIDs) == 0 {
		return nil, nil
	}
	args := []any{collectionID, pq.Array(docIDs)}
	pred, predArgs := asOfPredicate(span, len(args)+1)
	args = append(args, predArgs...)
	acl, aclArgs := docBothEndpointsVisibleClause(ctx, "$1", len(args)+1)
	args = append(args, aclArgs...)
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT doc_id, target_doc_id, link_type, relation, weight_f
		 FROM knowledge_links
		 WHERE collection_id = $1 AND link_type <> 'co_activated'
		   AND (doc_id = ANY($2) OR target_doc_id = ANY($2))
		   AND `+pred+` `+acl,
		args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.ActiveLink
	for rows.Next() {
		var l bizknowledge.ActiveLink
		if err := rows.Scan(&l.DocID, &l.TargetDocID, &l.LinkType, &l.Relation, &l.WeightF); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, l)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}

// ListCollectionLinksAsOf 时态版集合边读。投影/ACL/linkTypes 过滤/id 升序与
// ListCollectionLinks 一致；span 零值字节级委托原函数（现行行为：不做
// valid_to 过滤，closed 边照常返回），非零按四象限且排除 co_activated。
func (r *knowledgeRepo) ListCollectionLinksAsOf(ctx context.Context, collectionID string, linkTypes []string, span bizknowledge.TemporalSpan) ([]bizknowledge.Link, error) {
	if !span.Any() {
		return r.ListCollectionLinks(ctx, collectionID, linkTypes)
	}
	q := `SELECT id, collection_id, doc_id, target_doc_id, link_type, context, weight
		FROM knowledge_links
		WHERE collection_id = $1 AND link_type <> 'co_activated'`
	args := []any{collectionID}
	if len(linkTypes) > 0 {
		q += fmt.Sprintf(` AND link_type = ANY($%d)`, len(args)+1)
		args = append(args, pq.Array(linkTypes))
	}
	pred, predArgs := asOfPredicate(span, len(args)+1)
	q += ` AND ` + pred
	args = append(args, predArgs...)
	acl, aclArgs := docBothEndpointsVisibleClause(ctx, "$1", len(args)+1)
	q += " " + acl
	args = append(args, aclArgs...)
	q += ` ORDER BY id`
	rows, err := r.data.PostgresRead().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []bizknowledge.Link
	for rows.Next() {
		var l bizknowledge.Link
		if err := rows.Scan(&l.ID, &l.CollectionID, &l.DocID, &l.TargetDocID, &l.LinkType, &l.Context, &l.Weight); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out = append(out, l)
	}
	return out, entErrToBizErr(rows.Err(), "knowledge")
}
