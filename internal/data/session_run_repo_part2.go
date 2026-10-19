package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/sessionmetrics"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"time"
)

// MarkOrphanedRunsCancelled uses Raw SQL because it does a bulk conditional UPDATE
// with a WHERE clause that is not easily expressible via Ent's Update API.
//
// B01 fix: durable runs with valid checkpoints are preserved (they will be
// resumed by SessionRunDurableWorker). Only durable runs without a checkpoint
// (anomalous data) are cleaned up alongside interactive orphans.
//
// FIT-OBS-1: this path stamps finished_at AND increments session_metrics.error_count
// per affected session (orphaned runs never go through RecordTurnUsage).
func (r *sessionRunRepo) MarkOrphanedRunsCancelled(ctx context.Context) (int, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return 0, nil
	}
	now := biz.ChannelTurnJobNow()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE session_runs SET phase=?, error_message='orphaned: process restarted', finished_at=?, phase_changed_at=?, updated_at=?
WHERE (
    phase IN ('interactive')
    OR (phase='durable' AND (checkpoint_id IS NULL OR checkpoint_id=''))
  )
  AND (finished_at IS NULL OR finished_at='')
RETURNING session_id`),
		biz.SessionRunPhaseCancelled, now, now, nowStr,
	)
	if err != nil {
		r.data.lg.Error("mark orphaned runs cancelled failed", loggateway.StepID("data.session_run.orphan_cleanup"), loggateway.Err(err))
		return 0, entErrToBizErr(err, "SESSION_RUN")
	}
	defer rows.Close()
	var sessionIDs []string
	for rows.Next() {
		var sid string
		if scanErr := rows.Scan(&sid); scanErr != nil {
			r.data.lg.Warn("orphan cleanup scan session_id failed", loggateway.StepID("data.session_run.orphan_cleanup"), loggateway.Err(scanErr))
			continue
		}
		sessionIDs = append(sessionIDs, sid)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		r.data.lg.Warn("orphan cleanup rows error", loggateway.StepID("data.session_run.orphan_cleanup"), loggateway.Err(rowsErr))
	}
	n := len(sessionIDs)
	r.bumpOrphanErrorCounts(ctx, sessionIDs, nowStr)
	return n, nil
}

func countOrphanSessions(sessionIDs []string) map[string]int {
	out := make(map[string]int, len(sessionIDs))
	for _, id := range sessionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id]++
	}
	return out
}

func (r *sessionRunRepo) bumpOrphanErrorCounts(ctx context.Context, sessionIDs []string, finishedAt string) {
	counts := countOrphanSessions(sessionIDs)
	for sid, n := range counts {
		if err := biz.CheckRunHonesty("orphaned", n, finishedAt); err != nil {
			r.data.lg.Warn("FIT-OBS-1: orphaned run honesty missed",
				loggateway.StepID("data.session_run.orphan_cleanup"),
				loggateway.Err(err),
				loggateway.Str("session_id", sid))
		}
		c := r.writeClient(ctx)
		if c == nil {
			continue
		}
		if err := c.SessionMetrics.Create().
			SetID(sid).
			SetErrorCount(n).
			SetUpdatedAt(finishedAt).
			OnConflictColumns(sessionmetrics.FieldID).
			AddErrorCount(n).
			UpdateUpdatedAt().
			Exec(ctx); err != nil {
			r.data.lg.Warn("orphan error_count bump failed",
				loggateway.StepID("data.session_run.orphan_cleanup"),
				loggateway.Str("session_id", sid),
				loggateway.Err(err))
		}
	}
}
