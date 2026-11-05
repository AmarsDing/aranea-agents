-- Version 20261306: twin_openapi_idempotency（三仓联动 P0 底座安全前置，
-- 2026-11-02）——机器面（TwinOpenAPI /api/v1/*）run 提交的幂等键持久化。
-- 背景：handleCreateRun 原幂等表为进程内 map（重启即失效），HotCraftFlow
-- AGT / NovaTrader harness 的消费方契约「idempotency_key 天然 at-least-once
-- 安全」只在进程生命周期内成立，底座重启窗口内的重试会产出重复 run。
-- 本表把 (caller, idem_key) → run_id 落库，Store 为 INSERT ... ON CONFLICT
-- DO NOTHING（先到者赢，严禁 UPDATE 翻案，与 executor_waiter_decisions
-- 同律）；Lookup 命中直接返回原 run_id，不再触发图执行。
-- caller 维度 = 多调用方 token（ARANEA_TWINOPENAPI_CALLERS）的 name；
-- 存量单 token 部署 caller 恒为 "twinmonitor"，行为字节级不变。
-- created_at 定宽 RFC3339 UTC 文本布局（字典序==时序），lazy GC：
-- Store 同事务删除早于 7 天的行（幂等键语义窗口 = 消费方重试/对账窗口，
-- 远超 webhook 投递与 run 终态到达时间）。旧二进制对本表零读零写。
-- 仅建表：不加 ent schema、不播种。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS twin_openapi_idempotency (
  caller     TEXT NOT NULL,
  idem_key   TEXT NOT NULL,
  run_id     TEXT NOT NULL,
  graph_id   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (caller, idem_key)
);
