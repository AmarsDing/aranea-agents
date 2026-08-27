package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// memoryFactPendingRepo is the raw-SQL dual-dialect implementation of
// biz.MemoryFactPendingStore (79-runtime-governance R3). Table is created by
// DDL migration 20261249; writes are idempotent on id so worker retries never
// duplicate pending rows.
type memoryFactPendingRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.MemoryFactPendingStore = (*memoryFactPendingRepo)(nil)

// NewMemoryFactPendingRepo constructs the pending store; nil when DB absent.
func NewMemoryFactPendingRepo(data *Data, lg loggateway.Logger) biz.MemoryFactPendingStore {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &memoryFactPendingRepo{data: data, lg: lg.With(loggateway.Domain("memory_fact_pending_repo"))}
}

// NewMemoryFactPendingRepoFromData is the Wire-friendly constructor.
func NewMemoryFactPendingRepoFromData(d *Data) biz.MemoryFactPendingStore {
	if d == nil {
		return nil
	}
	return NewMemoryFactPendingRepo(d, d.lg)
}

const memoryFactPendingCols = "id, agent_id, fact_key, verdict, proposed_body, prior_body, " +
	"adjudicator_reason, payload_json, status, approver, created_at, decided_at"

// InsertPending persists a withheld write (idempotent on id).
func (r *memoryFactPendingRepo) InsertPending(ctx context.Context, rec biz.MemoryFactPendingRecord) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(rec.ID) == "" {
		return nil
	}
	d := r.data.Dialect()
	q := d.BuildInsertOrIgnore("memory_fact_pending", memoryFactPendingCols, d.Placeholders(12), "id")
	if rec.Status == "" {
		rec.Status = biz.MemoryFactPendingStatusPending
	}
	_, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q,
		rec.ID, rec.AgentID, rec.FactKey, rec.Verdict, rec.ProposedBody, rec.PriorBody,
		rec.AdjudicatorReason, rec.PayloadJSON, rec.Status, rec.Approver, rec.CreatedAt, rec.DecidedAt,
	)
	if err != nil {
		return entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	return nil
}

// GetPending returns one record by id; found=false when absent.
func (r *memoryFactPendingRepo) GetPending(ctx context.Context, id string) (biz.MemoryFactPendingRecord, bool, error) {
	var rec biz.MemoryFactPendingRecord
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(id) == "" {
		return rec, false, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + memoryFactPendingCols + " FROM memory_fact_pending WHERE id = ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, id)
	if err != nil {
		return rec, false, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	defer rows.Close()
	if !rows.Next() {
		return rec, false, nil
	}
	if err := scanMemoryFactPending(rows, &rec); err != nil {
		return rec, false, err
	}
	return rec, true, nil
}

// ListPending lists records newest first; empty agentID/status match all.
func (r *memoryFactPendingRepo) ListPending(ctx context.Context, agentID, status string, limit int) ([]biz.MemoryFactPendingRecord, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	var conds []string
	var args []any
	if s := strings.TrimSpace(agentID); s != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(status); s != "" {
		conds = append(conds, "status = ?")
		args = append(args, s)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	if limit <= 0 {
		limit = 50
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + memoryFactPendingCols + " FROM memory_fact_pending" + where +
			" ORDER BY created_at DESC, id DESC LIMIT ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	defer rows.Close()
	out := make([]biz.MemoryFactPendingRecord, 0, limit)
	for rows.Next() {
		var rec biz.MemoryFactPendingRecord
		if err := scanMemoryFactPending(rows, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	return out, nil
}

// MarkDecided transitions a pending row to its decision. Fail-closed: the
// WHERE status='pending' guard makes double decisions a no-op (applied=false).
func (r *memoryFactPendingRepo) MarkDecided(ctx context.Context, id, status, approver string, decidedAt int64) (bool, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(id) == "" {
		return false, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"UPDATE memory_fact_pending SET status = ?, approver = ?, decided_at = ? WHERE id = ? AND status = 'pending'")
	res, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q, status, approver, decidedAt, id)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	return n > 0, nil
}

var _ biz.MemoryFactPendingCounter = (*memoryFactPendingRepo)(nil)

// CountPendingByAge 对 status='pending' 行按 age 互斥分档 COUNT
// （79-runtime-governance R8 P5.2）。SUM(CASE WHEN) 双方言兼容（PG FILTER
// 语法 SQLite 不支持）。age = nowUnix - created_at（秒）；分档边界与
// diagnostics/audit.py 的严格 > 语义对齐：
//
//	staleFail: age > failAgeSec          → created_at <  now-fail
//	staleWarn: warnAgeSec < age <= fail  → now-fail <= created_at < now-warn
func (r *memoryFactPendingRepo) CountPendingByAge(ctx context.Context, warnAgeSec, failAgeSec, nowUnix int64) (int64, int64, int64, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return 0, 0, 0, nil
	}
	warnBound := nowUnix - warnAgeSec
	failBound := nowUnix - failAgeSec
	q := r.data.Dialect().RenumberPlaceholders(
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN created_at < ? THEN 1 ELSE 0 END), 0)
		   FROM memory_fact_pending WHERE status = 'pending'`)
	var total, staleWarn, staleFail int64
	found, err := scanUsageSingleRow(ctx, r.data.RWDB().ReadDB(ctx), q,
		[]any{failBound, warnBound, failBound}, &total, &staleWarn, &staleFail)
	if err != nil {
		return 0, 0, 0, entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	if !found {
		return 0, 0, 0, nil
	}
	return total, staleWarn, staleFail, nil
}

type memoryFactPendingScanner interface {
	Scan(dest ...any) error
}

func scanMemoryFactPending(row memoryFactPendingScanner, rec *biz.MemoryFactPendingRecord) error {
	if err := row.Scan(
		&rec.ID, &rec.AgentID, &rec.FactKey, &rec.Verdict, &rec.ProposedBody, &rec.PriorBody,
		&rec.AdjudicatorReason, &rec.PayloadJSON, &rec.Status, &rec.Approver, &rec.CreatedAt, &rec.DecidedAt,
	); err != nil {
		return entErrToBizErr(err, "MEMORY_FACT_PENDING")
	}
	return nil
}
