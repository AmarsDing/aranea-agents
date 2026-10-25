-- Version 20261302: executor_waiter_decisions（P0-6 阶段 1b，deferred 战役 D6，
-- 97-multi-replica-executor-waiters）
-- 跨副本执行等待器决议表：DecisionKey(kind, session_id, scope_id) 三列主键。
-- TryRecordDecision = INSERT ... ON CONFLICT DO NOTHING——0 行命中=别副本已
-- 决议，调用方 LoadDecision 一读拿现值；严禁 UPDATE 覆盖（防两管理员双副本
-- approve/deny 并发翻案：先到者赢，后到者保留先到决议）。
-- kind 取值：playbook_confirm（R18 阶段确认）、board_startup（P2-② 首次团队
-- 派发对账）、team_agent_loads（DEV-03 槽位快照，keys 以 "," 拼接入 reason）。
-- decision 布尔列承载 approve/ok；reason/actor 明文明列；decided_at 定宽布局
--（"2006-01-02T15:04:05.000000000Z07:00"，纳秒恒 9 位——board_run_leases
-- 教训同律，字典序==时序）。
-- 行生命周期（对齐 C-g/D5 记档格式）：不设 sweeper——行量≈决议次数
--（HITL 确认/看板启动各每看板至多一行，team 槽位快照每 team 至多一行），
-- 低频有界；看板清理链带删评估记档 97 三件套后续批。
-- 消费面全部藏在 ARANEA_EXECUTOR_WAITER=off|mem|pg（缺省 off 字节级旧行为）
-- 之后：off 档本表零写零读。仅建表：不加 ent schema、不播种。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS executor_waiter_decisions (
  kind       TEXT    NOT NULL,
  session_id TEXT    NOT NULL DEFAULT '',
  scope_id   TEXT    NOT NULL,
  decision   BOOLEAN NOT NULL DEFAULT FALSE,
  reason     TEXT    NOT NULL DEFAULT '',
  actor      TEXT    NOT NULL DEFAULT '',
  decided_at TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (kind, session_id, scope_id)
);
