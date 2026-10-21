-- 20261288 orchestration_cache_entries（D7-F5，2026-10-21）：编排配方缓存逐行持久化。
-- PK(workspace_id, task_pattern) 天然呈现租户分桶（D7-F6）；entry_json 承载完整
-- OrchestrationCacheEntry（含 AgentKeys/Specialties/provenance），dq_score/updated_at
-- 冗余列供列表排序与 TTL 巡检免 JSON 解析。legacy system_settings.orchestration_cache_json
-- 由应用层 InitFromRepo 一次性逐条导入本表后弃用，不在 SQL 层搬数据。
-- 注册版本 20261288：20261287 为 knowledge_links_recorded_idx 占用（合并重排，
-- 见 ddl_migration_table_late.go）；SQL 文件保留原磁盘文件名——版本唯一性由
-- 注册表 Version 字段保证（先例：20261279 保留 20261278 文件名）。

CREATE TABLE IF NOT EXISTS orchestration_cache_entries (
  workspace_id TEXT NOT NULL DEFAULT '',
  task_pattern TEXT NOT NULL,
  entry_json TEXT NOT NULL,
  dq_score REAL NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, task_pattern)
);

CREATE INDEX IF NOT EXISTS idx_orch_cache_entries_updated
  ON orchestration_cache_entries (workspace_id, updated_at);
