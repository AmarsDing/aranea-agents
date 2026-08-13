-- Version 20261210: plugin cost_guard 日预算持久化表（GAP-01 / I-2）
-- CostGuardBudgetTracker 经 AddTokens(INSERT ... ON CONFLICT(usage_day, scope_key) DO UPDATE)
-- 依赖本表存在；此前无 DDL 迁移注册，全新部署 cost_guard 持久化必败。
CREATE TABLE IF NOT EXISTS plugin_cost_guard_usage (
  usage_day TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  tokens INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (usage_day, scope_key)
);
