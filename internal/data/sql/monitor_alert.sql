-- Monitor alert rules (MON-01)

CREATE TABLE IF NOT EXISTS monitor_alert_rules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  metric_key TEXT NOT NULL,
  threshold REAL NOT NULL,
  window_minutes INTEGER NOT NULL DEFAULT 60,
  enabled INTEGER NOT NULL DEFAULT 1,
  severity TEXT NOT NULL DEFAULT 'warning',
  notify_webhook_url TEXT NOT NULL DEFAULT '',
  notify_channel_id TEXT NOT NULL DEFAULT '',
  cooldown_minutes INTEGER NOT NULL DEFAULT 60,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_monitor_alert_rules_enabled ON monitor_alert_rules(enabled);
