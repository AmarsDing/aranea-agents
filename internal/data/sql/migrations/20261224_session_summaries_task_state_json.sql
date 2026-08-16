-- Version 20261224: v4 压缩契约——session_summaries 加 task_state_json 列，
-- 承载压缩 LLM 产出的结构化任务状态段（叙事摘要之外的双段化产物）。
-- 幂等性由迁移运行器 executeSQLFileWithDialect 的 duplicate-column 容错保证
-- （与存量 ADD COLUMN 迁移同款模式）；SQLite 与 PostgreSQL 通用。
ALTER TABLE session_summaries ADD COLUMN task_state_json TEXT NOT NULL DEFAULT '';
