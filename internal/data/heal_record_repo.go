package data

import (
	"context"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
)

type healRecordRepo struct {
	data *Data
}

var _ bizmonitor.HealRecordRepo = (*healRecordRepo)(nil)

// NewHealRecordRepo creates a new HealRecordRepo backed by raw SQL.
func NewHealRecordRepo(d *Data) bizmonitor.HealRecordRepo {
	return &healRecordRepo{data: d}
}

func (r *healRecordRepo) InsertHealRecord(ctx context.Context, record bizmonitor.HealRecord) error {
	if r == nil || r.data == nil {
		return apierror.Internal("HEAL_RECORD", "database not configured")
	}

	d := r.data.Dialect()
	columns := "id, rule_id, trigger_type, trace_id, session_id, step_id, fix_action_type, confidence, status, runtime_auto_healed, runtime_heal_attempts, reason, created_at"
	placeholders := d.Placeholders(13)
	// SQLite: INSERT OR IGNORE INTO heal_records (...) VALUES (...)
	// Postgres: INSERT INTO heal_records (...) VALUES (...) ON CONFLICT (id) DO NOTHING
	stmt := d.BuildInsertOrIgnore("heal_records", columns, placeholders, "id")
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, stmt,
		record.ID,
		record.RuleID,
		record.TriggerType,
		record.TraceID,
		record.SessionID,
		record.StepID,
		record.FixAction.Type,
		record.Confidence,
		record.Status,
		record.RuntimeAutoHealed,
		record.RuntimeHealAttempts,
		record.Reason,
		record.CreatedAt,
	)
	return err
}

func (r *healRecordRepo) ListHealRecords(ctx context.Context, query bizmonitor.HealRecordQuery) (bizmonitor.HealRecordListResult, error) {
	if r == nil || r.data == nil {
		return bizmonitor.HealRecordListResult{}, apierror.Internal("HEAL_RECORD", "database not configured")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	where, args := healRecordWhere(query)
	d := r.data.Dialect()
	countSQL := d.RenumberPlaceholders(`SELECT COUNT(*) FROM heal_records` + where)
	listSQL := d.RenumberPlaceholders(`SELECT id, rule_id, trigger_type, trace_id, session_id, step_id,
		fix_action_type, confidence, status, runtime_auto_healed, runtime_heal_attempts, reason, created_at
		FROM heal_records` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`)
	args = append(args, limit, offset)

	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args[:len(args)-2], &total); err != nil {
		return bizmonitor.HealRecordListResult{}, entErrToBizErr(err, "HEAL_RECORD")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, listSQL, args...)
	if err != nil {
		return bizmonitor.HealRecordListResult{}, entErrToBizErr(err, "HEAL_RECORD")
	}
	defer rows.Close()

	var items []bizmonitor.HealRecord
	for rows.Next() {
		var (
			id, ruleID, triggerType, traceID, sessionID, stepID string
			fixActionType, status, reason, createdAt            string
			confidence                                          float64
			runtimeAutoHealed                                   bool
			runtimeHealAttempts                                 int
		)
		if err := rows.Scan(&id, &ruleID, &triggerType, &traceID, &sessionID, &stepID,
			&fixActionType, &confidence, &status, &runtimeAutoHealed, &runtimeHealAttempts, &reason, &createdAt); err != nil {
			return bizmonitor.HealRecordListResult{}, entErrToBizErr(err, "HEAL_RECORD")
		}
		items = append(items, bizmonitor.HealRecord{
			ID: id, RuleID: ruleID, TriggerType: triggerType,
			TraceID: traceID, SessionID: sessionID, StepID: stepID,
			FixAction:           bizmonitor.FixAction{Type: fixActionType},
			Confidence:          confidence,
			Status:              status,
			RuntimeAutoHealed:   runtimeAutoHealed,
			RuntimeHealAttempts: runtimeHealAttempts,
			Reason:              reason,
			CreatedAt:           createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return bizmonitor.HealRecordListResult{}, entErrToBizErr(err, "HEAL_RECORD")
	}

	return bizmonitor.HealRecordListResult{Items: items, Total: total}, nil
}

func (r *healRecordRepo) DeleteHealRecordsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	if r == nil || r.data == nil {
		return 0, apierror.Internal("HEAL_RECORD", "database not configured")
	}

	cutoff := olderThan.Format(time.RFC3339)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`DELETE FROM heal_records WHERE created_at < ?`), cutoff)
	if err != nil {
		return 0, entErrToBizErr(err, "HEAL_RECORD")
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func healRecordWhere(q bizmonitor.HealRecordQuery) (string, []any) {
	var conds []string
	var args []any
	if q.RuleID != "" {
		conds = append(conds, "rule_id = ?")
		args = append(args, q.RuleID)
	}
	if q.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, q.Status)
	}
	if q.SessionID != "" {
		conds = append(conds, "session_id = ?")
		args = append(args, q.SessionID)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + joinConds(conds, " AND ")
	}
	return where, args
}

func joinConds(conds []string, sep string) string {
	if len(conds) == 0 {
		return ""
	}
	result := conds[0]
	for _, c := range conds[1:] {
		result += sep + c
	}
	return result
}
