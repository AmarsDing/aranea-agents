package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"
)

// decisionQueryRepo 是 decision.QueryRepo 的 raw-SQL 双方言实现
// （M80 Phase 1 查询面，设计 §4.2）。与 decision_repo.go 同表不同型：
// 写侧为幂等 insert/outbox，读侧为过滤分页查询。JSON 过滤走 Dialect
// 辅助（SQLite json_extract/json_each，PG ->>/json_array_elements），
// 全部条件下推到 DB 侧，禁全表扫。
type decisionQueryRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ decision.QueryRepo = (*decisionQueryRepo)(nil)

// NewDecisionQueryRepoFromData is the Wire-friendly constructor; nil when
// DB is absent (CLI mode) — QueryUsecase treats a nil repo as empty results.
func NewDecisionQueryRepoFromData(d *Data) decision.QueryRepo {
	if d == nil || d.RWDB() == nil {
		return nil
	}
	lg := d.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &decisionQueryRepo{data: d, lg: lg.With(loggateway.Domain("decision_query_repo"))}
}

const decisionRecordSelectCols = "id, decision_key, category, scenario, reasoning, outcome, confidence, " +
	"actor_type, actor_key, parent_decision_id, related_entities, source_ref, metadata, " +
	"workspace_id, created_at, updated_at"

// buildDecisionListWhere 组装 WHERE 子句与参数（? 占位，方言侧统一重编号）。
func buildDecisionListWhere(d Dialect, f decision.ListFilter) (string, []any) {
	var conds []string
	var args []any
	if s := strings.TrimSpace(f.Category); s != "" {
		conds = append(conds, "category = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.ActorKey); s != "" {
		conds = append(conds, "actor_key = ?")
		args = append(args, s)
	}
	// 实体过滤：related_entities 数组内对象 {type,key} 成对匹配。
	// PG 走 @> 包含且表达式必须是裸 related_entities::jsonb——与 20261252
	// entities GIN 索引表达式精确一致方可命中（COALESCE 防御壳会导致索引
	// 失配全表扫，1.10 实测 1.28s/276ms 两连踩）；列 NOT NULL DEFAULT '[]'
	// 且写入侧恒为 json.Marshal 产物，裸转换安全。语义与 jsonb_array_elements
	// 成对比较等价（数组任一元素含该 type+key 即中）。SQLite 保留 json_each。
	if strings.TrimSpace(f.EntityType) != "" && strings.TrimSpace(f.EntityKey) != "" {
		if d.IsPostgres() {
			conds = append(conds, `related_entities::jsonb @> ?::jsonb`)
			containment, err := json.Marshal([]decision.EntityRef{
				{Type: strings.TrimSpace(f.EntityType), Key: strings.TrimSpace(f.EntityKey)},
			})
			if err != nil {
				return "", nil
			}
			args = append(args, string(containment))
		} else {
			conds = append(conds, `EXISTS (SELECT 1 FROM json_each(related_entities) je `+
				`WHERE json_extract(je.value, '$.type') = ? AND json_extract(je.value, '$.key') = ?)`)
			args = append(args, strings.TrimSpace(f.EntityType), strings.TrimSpace(f.EntityKey))
		}
	}
	// source_run_id 过滤：source_ref.run_id（btree 表达式索引见 20261254）。
	if s := strings.TrimSpace(f.SourceRunID); s != "" {
		conds = append(conds, d.JSONExtract("source_ref", "run_id")+" = ?")
		args = append(args, s)
	}
	if !f.TimeFrom.IsZero() {
		conds = append(conds, "created_at >= ?")
		args = append(args, f.TimeFrom.UTC().Format(time.RFC3339Nano))
	}
	if !f.TimeTo.IsZero() {
		conds = append(conds, "created_at <= ?")
		args = append(args, f.TimeTo.UTC().Format(time.RFC3339Nano))
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListRecords 按过滤条件分页查询（created_at DESC, id DESC 稳定序）。
func (r *decisionQueryRepo) ListRecords(ctx context.Context, f decision.ListFilter) ([]decision.Record, int64, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, 0, nil
	}
	d := r.data.Dialect()
	where, args := buildDecisionListWhere(d, f)
	limit, offset := f.PageSize, (f.Page-1)*f.PageSize
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	countQ := d.RenumberPlaceholders("SELECT COUNT(*) FROM decision_records" + where)
	var total int64
	if err := QueryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countQ, args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "DECISION")
	}

	listQ := d.RenumberPlaceholders(
		"SELECT " + decisionRecordSelectCols + " FROM decision_records" + where +
			" ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?")
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	items := make([]decision.Record, 0, limit)
	for rows.Next() {
		rec, err := scanDecisionRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, entErrToBizErr(err, "DECISION")
	}
	return items, total, nil
}

// GetByKey 按 decision_key 精确查询；未命中返回 nil, nil。
func (r *decisionQueryRepo) GetByKey(ctx context.Context, decisionKey string) (*decision.Record, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(decisionKey) == "" {
		return nil, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + decisionRecordSelectCols + " FROM decision_records WHERE decision_key = ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, decisionKey)
	if err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	rec, err := scanDecisionRecord(rows)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// decisionRowScanner 抽象 *sql.Rows / *sql.Row 的 Scan。
type decisionRowScanner interface {
	Scan(dest ...any) error
}

// scanDecisionRecord 把一行 decision_records 扫描为 biz Record
// （JSON 文本列反序列化；可空列 confidence/parent_decision_id 归位指针）。
func scanDecisionRecord(row decisionRowScanner) (*decision.Record, error) {
	var rec decision.Record
	var confidence sql.NullFloat64
	var parentID sql.NullInt64
	var entities, sourceRef, metadata string
	var category, actorType string // 自定义类型须经 string 过渡（database/sql 不认命名类型）
	if err := row.Scan(&rec.ID, &rec.DecisionKey, &category, &rec.Scenario, &rec.Reasoning,
		&rec.Outcome, &confidence, &actorType, &rec.ActorKey, &parentID,
		&entities, &sourceRef, &metadata, &rec.WorkspaceID, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	rec.Category = decision.Category(category)
	rec.ActorType = decision.ActorType(actorType)
	if confidence.Valid {
		v := confidence.Float64
		rec.Confidence = &v
	}
	if parentID.Valid {
		v := parentID.Int64
		rec.ParentDecisionID = &v
	}
	if err := json.Unmarshal([]byte(entities), &rec.RelatedEntities); err != nil {
		return nil, entErrToBizErr(fmt.Errorf("decision_records.related_entities decode: %w", err), "DECISION")
	}
	if err := json.Unmarshal([]byte(sourceRef), &rec.SourceRef); err != nil {
		return nil, entErrToBizErr(fmt.Errorf("decision_records.source_ref decode: %w", err), "DECISION")
	}
	if metadata != "" {
		if err := json.Unmarshal([]byte(metadata), &rec.Metadata); err != nil {
			return nil, entErrToBizErr(fmt.Errorf("decision_records.metadata decode: %w", err), "DECISION")
		}
	}
	return &rec, nil
}
