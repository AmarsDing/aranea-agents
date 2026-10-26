-- Version 20261301: knowledge_source_connector_runs（G-38 SourceSync W1，
-- 轮次史表——runs 弥补 cron_task_run 无逐轮历史的欠账目）
-- finalize 终态单行：六计数（created/updated/unchanged/fetch_failed/deleted/
-- miss_unknown）+ bytes_pulled（⑤ 成本观测径）+ 稳定 error_code/截断 message。
-- P10 拍板 a 硬条文：finalize 同事务单行 DELETE 保每 connector 最近 100 轮
--（常量钉死，cron_task_run 无 GC 教训的对冲设计）；姊妹条款同事务第二
-- DELETE 清超 90 天 missing/deleted 指纹行（while W1 落 SQL 形，W7 起钉）。
-- idx_src_runs_conn (connector_id, started_at DESC) 是保留 DELETE 与
-- runs 列表查询（⑦ GET /{id}/runs?limit）的共用硬前置索引。
-- 运行中无行（在途态=connectors.run_token 承载）；status 四值全为终态。
CREATE TABLE IF NOT EXISTS knowledge_source_connector_runs (
  id TEXT PRIMARY KEY,
  connector_id TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('success', 'partial', 'failed', 'skipped')),
  created INTEGER NOT NULL DEFAULT 0,
  updated INTEGER NOT NULL DEFAULT 0,
  unchanged INTEGER NOT NULL DEFAULT 0,
  fetch_failed INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0,
  miss_unknown INTEGER NOT NULL DEFAULT 0,
  bytes_pulled BIGINT NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT ''
);
-- P10 同批硬前置：保留裁剪与倒序列举共用。
CREATE INDEX IF NOT EXISTS idx_src_runs_conn
  ON knowledge_source_connector_runs (connector_id, started_at DESC);
