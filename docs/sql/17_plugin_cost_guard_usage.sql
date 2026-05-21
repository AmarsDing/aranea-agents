-- cost_guard daily token budget (cross-process persistence)
CREATE TABLE IF NOT EXISTS plugin_cost_guard_usage (
  usage_day TEXT NOT NULL,
  scope_key TEXT NOT NULL DEFAULT 'global',
  tokens INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (usage_day, scope_key)
);
