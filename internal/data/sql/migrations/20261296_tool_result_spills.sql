-- Version 20261296: tool_result_spills（C-5 PG SpillStore，91-multi-replica-spill-store）
-- P0-2 SpillStore 抽象的 PG 第二实现：LocalFS 单副本边界在多副本部署下
-- locator 跨副本 404（P0-6 阶段 1a 定案后解锁立项）。content BYTEA 原字节
-- 承载（单文件 conf 默认上界 10MiB 远低于 PG 行限）；session_hash =
-- sha256(sessionID) hex[:16] 行过滤键（不落原 sessionID，跨会话防读同
-- LocalFS resolveOwned 的 ErrNotFound 姿态）；created_at TEXT + 定宽布局
-- 2006-01-02T15:04:05.000000000Z07:00（批C C-e #0/#11 教训：RFC3339Nano
-- 变宽小数在整秒瞬时字典序倒置，TEXT 时间列做范围比较必须定宽）。
-- (session_hash, created_at) 联合索引仅服务 Put 的会话配额 SUM 扫描
-- （leading 列等值）。GC 两条热语句（TTL DELETE 与 oldest-first 全表序）
-- 均不带 session_hash 谓词，复合索引无法驱动（PG 无 skip-scan）——C-f
-- SPILL-PG-01/GC-3 裁断：GC 扫表为刻意选择，wire 缺省 MaxTotalBytes=64MiB
-- 使表量有界、seq scan 代价固定；总量开闸（MaxTotalBytes=0 直连构造）
-- 且 GC 调度接线（deferred Q7）时再评估单列 created_at 索引。
-- id = 32 hex chars（8B 毫秒 + 8B crypto/rand，128bit 空间碰撞可忽略，
-- Put 契约钉死冲突重试一次）。
-- 编号顺延当时 migrations 最大值（20261295 board_run_leases）+1，无撞号。
-- 默认 capability flag 关闭（ARANEA_TOOLSPILL_STORE 未设=fs 档字节级旧
-- 行为），本表仅 env=pg 时消费。仅建表：不加 ent schema、不播种。
-- 幂等，重跑安全。
CREATE TABLE IF NOT EXISTS tool_result_spills (
  id           TEXT PRIMARY KEY,
  session_hash TEXT NOT NULL DEFAULT '',
  tool         TEXT NOT NULL DEFAULT '',
  content      BYTEA NOT NULL,
  byte_size    BIGINT NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_tool_result_spills_session_created
  ON tool_result_spills(session_hash, created_at);
