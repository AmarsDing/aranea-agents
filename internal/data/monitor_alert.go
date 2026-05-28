package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event"

	"github.com/google/uuid"
)

// ensureMonitorAlertFiringStateCols adds the MON-OPT-02 columns to monitor_alert_rules
// exactly once per process lifetime (H-04: was running PRAGMA on every ListAlertRules call).
func (r *monitorRepo) ensureMonitorAlertFiringStateCols(ctx context.Context) {
	r.firingColsOnce.Do(func() {
		db := r.data.RawDB()
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(monitor_alert_rules)`)
		if err != nil {
			event.SysLogWarn("system.monitor.alert_count_fail", "ensureMonitorAlertFiringStateCols: PRAGMA failed", event.P("error", err.Error()))
			return
		}
		existing := map[string]bool{}
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err == nil {
				existing[name] = true
			}
		}
		rows.Close()

		alters := []string{
			`ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_at INTEGER`,
			`ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_value REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_window_start INTEGER`,
			`ALTER TABLE monitor_alert_rules ADD COLUMN firing_state TEXT NOT NULL DEFAULT 'idle'`,
			`ALTER TABLE monitor_alert_rules ADD COLUMN recovered_at INTEGER`,
		}
		cols := []string{"last_fired_at", "last_fired_value", "last_fired_window_start", "firing_state", "recovered_at"}
		for i, col := range cols {
			if existing[col] {
				continue
			}
			if _, err := db.ExecContext(ctx, alters[i]); err != nil {
				event.SysLogWarn("system.monitor.alert_count_fail", "ensureMonitorAlertFiringStateCols: ALTER TABLE failed", event.P("col", col), event.P("error", err.Error()))
			}
		}
	})
}

func (r *monitorRepo) ListAlertRules(ctx context.Context) ([]biz.MonitorAlertRule, error) {
	r.ensureMonitorAlertFiringStateCols(ctx)
	rows, err := r.data.RawDB().QueryContext(ctx, `
SELECT id, name, metric_key, threshold, window_minutes, enabled, severity,
       COALESCE(notify_webhook_url,''), COALESCE(notify_channel_id,''), COALESCE(cooldown_minutes,60),
       created_at, updated_at,
       COALESCE(firing_state,'idle'), last_fired_at, COALESCE(last_fired_value,0),
       last_fired_window_start, recovered_at
FROM monitor_alert_rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.MonitorAlertRule
	for rows.Next() {
		var rule biz.MonitorAlertRule
		var enabled int
		var firingState string
		var lastFiredAtMs, lastFiredWindowStartMs, recoveredAtMs sql.NullInt64
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.MetricKey, &rule.Threshold, &rule.WindowMinutes, &enabled, &rule.Severity,
			&rule.NotifyWebhookURL, &rule.NotifyChannelID, &rule.CooldownMinutes, &rule.CreatedAt, &rule.UpdatedAt,
			&firingState, &lastFiredAtMs, &rule.LastFiredValue,
			&lastFiredWindowStartMs, &recoveredAtMs,
		); err != nil {
			return nil, err
		}
		rule.Enabled = enabled != 0
		rule.FiringState = monitor.AlertFiringState(firingState)
		if lastFiredAtMs.Valid {
			t := time.UnixMilli(lastFiredAtMs.Int64).UTC()
			rule.LastFiredAt = &t
		}
		if lastFiredWindowStartMs.Valid {
			t := time.UnixMilli(lastFiredWindowStartMs.Int64).UTC()
			rule.LastFiredWindowStart = &t
		}
		if recoveredAtMs.Valid {
			t := time.UnixMilli(recoveredAtMs.Int64).UTC()
			rule.RecoveredAt = &t
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *monitorRepo) ReplaceAlertRules(ctx context.Context, rules []biz.MonitorAlertRule) error {
	r.ensureMonitorAlertFiringStateCols(ctx)
	tx, err := r.data.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM monitor_alert_rules`); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, rule := range rules {
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			id = uuid.NewString()
		}
		enabled := 0
		if rule.Enabled {
			enabled = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO monitor_alert_rules
  (id, name, metric_key, threshold, window_minutes, enabled, severity,
   notify_webhook_url, notify_channel_id, cooldown_minutes, created_at, updated_at,
   firing_state)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,'idle')`,
			id, rule.Name, rule.MetricKey, rule.Threshold, rule.WindowMinutes, enabled, rule.Severity,
			rule.NotifyWebhookURL, rule.NotifyChannelID, rule.CooldownMinutes, now, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateAlertFiringState persists the firing state machine columns (MON-OPT-02).
//
// SQLite single-writer guarantee: we run BEGIN IMMEDIATE so that any concurrent
// reader that also tries to fire the same rule is blocked until this transaction
// commits. This prevents duplicate Webhook notifications on restart or multi-goroutine
// evaluation runs.
func (r *monitorRepo) UpdateAlertFiringState(
	ctx context.Context,
	id string,
	state monitor.AlertFiringState,
	lastFiredAt *time.Time,
	lastFiredValue float64,
	recoveredAt *time.Time,
) error {
	r.ensureMonitorAlertFiringStateCols(ctx)
	db := r.data.RawDB()

	// BEGIN IMMEDIATE acquires a write lock immediately; BEGIN DEFERRED (the default)
	// only upgrades to write on the first write statement, leaving a gap for races.
	if _, err := db.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("UpdateAlertFiringState BEGIN IMMEDIATE: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			db.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		}
	}()

	var lastFiredAtMs sql.NullInt64
	if lastFiredAt != nil {
		lastFiredAtMs = sql.NullInt64{Int64: lastFiredAt.UnixMilli(), Valid: true}
	}
	var recoveredAtMs sql.NullInt64
	if recoveredAt != nil {
		recoveredAtMs = sql.NullInt64{Int64: recoveredAt.UnixMilli(), Valid: true}
	}

	if _, err := db.ExecContext(ctx, `
UPDATE monitor_alert_rules
SET firing_state = ?, last_fired_at = ?, last_fired_value = ?, recovered_at = ?,
    updated_at = ?
WHERE id = ?`,
		string(state), lastFiredAtMs, lastFiredValue, recoveredAtMs,
		time.Now().UTC().Format(time.RFC3339), id,
	); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("UpdateAlertFiringState COMMIT: %w", err)
	}
	committed = true
	return nil
}

func (r *monitorRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status string, sinceRFC3339 string) (int32, error) {
	q := `SELECT COUNT(*) FROM monitor_events WHERE deleted_at = '' AND event_key = ? AND created_at >= ?`
	args := []any{eventKey, sinceRFC3339}
	if strings.TrimSpace(status) != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	var n int32
	err := r.data.RawDB().QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

func (r *monitorRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	var avg sql.NullFloat64
	err := r.data.RawDB().QueryRowContext(ctx, `
SELECT AVG(CAST(COALESCE(meta_duration_ms, json_extract(metadata_json, '$.duration_ms')) AS REAL))
FROM monitor_events
WHERE deleted_at = '' AND event_key = 'runner.completion' AND created_at >= ?
  AND COALESCE(meta_duration_ms, json_extract(metadata_json, '$.duration_ms')) IS NOT NULL`, sinceRFC3339).Scan(&avg)
	if err != nil {
		return 0, err
	}
	if !avg.Valid {
		return 0, nil
	}
	return avg.Float64, nil
}

func (r *monitorRepo) LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (p50, p95, p99 float64, err error) {
	rows, qErr := r.data.RawDB().QueryContext(ctx, `
SELECT CAST(COALESCE(meta_duration_ms, json_extract(metadata_json, '$.duration_ms')) AS REAL) AS dur
FROM monitor_events
WHERE deleted_at = '' AND event_key = 'runner.completion' AND created_at >= ?
  AND COALESCE(meta_duration_ms, json_extract(metadata_json, '$.duration_ms')) IS NOT NULL
ORDER BY dur ASC`, sinceRFC3339)
	if qErr != nil {
		return 0, 0, 0, qErr
	}
	defer rows.Close()
	var durations []float64
	for rows.Next() {
		var d float64
		if scanErr := rows.Scan(&d); scanErr != nil {
			continue
		}
		if d > 0 {
			durations = append(durations, d)
		}
	}
	n := len(durations)
	if n == 0 {
		return 0, 0, 0, nil
	}
	p50 = durations[percentileIndex(n, 50)]
	p95 = durations[percentileIndex(n, 95)]
	p99 = durations[percentileIndex(n, 99)]
	return p50, p95, p99, nil
}

func percentileIndex(n, percentile int) int {
	if n <= 1 {
		return 0
	}
	idx := float64(percentile) / 100.0 * float64(n-1)
	lo := int(idx)
	if lo >= n-1 {
		return n - 1
	}
	return lo
}
