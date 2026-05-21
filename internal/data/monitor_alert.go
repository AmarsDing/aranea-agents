package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

func (r *monitorRepo) ListAlertRules(ctx context.Context) ([]biz.MonitorAlertRule, error) {
	rows, err := r.data.RawDB().QueryContext(ctx, `
SELECT id, name, metric_key, threshold, window_minutes, enabled, severity,
       COALESCE(notify_webhook_url,''), COALESCE(notify_channel_id,''), COALESCE(cooldown_minutes,60),
       created_at, updated_at
FROM monitor_alert_rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.MonitorAlertRule
	for rows.Next() {
		var rule biz.MonitorAlertRule
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.MetricKey, &rule.Threshold, &rule.WindowMinutes, &enabled, &rule.Severity,
			&rule.NotifyWebhookURL, &rule.NotifyChannelID, &rule.CooldownMinutes, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled != 0
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *monitorRepo) ReplaceAlertRules(ctx context.Context, rules []biz.MonitorAlertRule) error {
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
INSERT INTO monitor_alert_rules (id, name, metric_key, threshold, window_minutes, enabled, severity,
  notify_webhook_url, notify_channel_id, cooldown_minutes, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, rule.Name, rule.MetricKey, rule.Threshold, rule.WindowMinutes, enabled, rule.Severity,
			rule.NotifyWebhookURL, rule.NotifyChannelID, rule.CooldownMinutes, now, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
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
SELECT AVG(CAST(json_extract(metadata_json, '$.duration_ms') AS REAL))
FROM monitor_events
WHERE deleted_at = '' AND event_key = 'runner.completion' AND created_at >= ?
  AND json_extract(metadata_json, '$.duration_ms') IS NOT NULL`, sinceRFC3339).Scan(&avg)
	if err != nil {
		return 0, err
	}
	if !avg.Valid {
		return 0, nil
	}
	return avg.Float64, nil
}
