-- Version 20261295: board_run_leases（P0-6 阶段 1a，90-multi-replica-board-lease）
-- PlanBoard DAG 执行租约的 PG 行锁表：board_id 主键 + owner_id/expires_at
-- 心跳取址。多副本下同一 PlanBoardCreatedEvent 只被一行租约放行——
-- TryClaim 走 INSERT ... ON CONFLICT DO UPDATE ... WHERE expires_at<=now
--（仅过期收养，owner 语义由 service 侧 PGBoardRunLease 心跳维护），
-- (session_id, expires_at) 索引支撑 HasActiveForSession 波次中点跨副本门控。
-- 时间列沿用 channel_runtime_lease 先例（TEXT + RFC3339Nano，driver 无关）。
-- 编号顺延当时 migrations 最大值（20261294 document_versions_chunk_archive）+1，
-- 无撞号。默认 capability flag 关闭（ARANEA_BOARD_RUN_LEASE 未设=纯进程内
-- boardRunRegistry 字节级旧行为），PG 行租约仅 flag=pg 时消费。
-- 仅建表：不加 ent schema、不播种。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS board_run_leases (
  board_id   TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  owner_id   TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_board_run_leases_session
  ON board_run_leases(session_id, expires_at);
