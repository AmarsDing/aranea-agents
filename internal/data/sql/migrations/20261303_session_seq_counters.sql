-- Version 20261303: session_seq_counters（P0-6 阶段 2，deferred 战役 D7-1，
-- 98-multi-replica-outbox-seq-claim）
-- 每会话关键事件 outbox 行 delivery seq 的持久计数器。取号语句：
--   INSERT INTO session_seq_counters (session_id, next_seq)
--   VALUES (?, (SELECT COALESCE(MAX(seq),0)+2 FROM event_delivery_outbox
--               WHERE session_id=?))
--   ON CONFLICT (session_id) DO UPDATE
--     SET next_seq = session_seq_counters.next_seq + 1
--   RETURNING next_seq - 1
-- 与 outbox INSERT 同处一个事务：并发副本在计数器行锁上串行，取号序==
-- 提交序==可见序——last_event_id 严格游标回放（ListAfter seq>cursor）永不
-- 见到 N+1 先于 N 可见（双副本进程内计数器号段倒置的消除案）。
-- 首分配种子 MAX(seq)+2（而非 2）：flag 从 off 翻到 pg 时既有会话的存量
-- outbox 行（Go 计数器产出）不被撞号；混版本滚动窗口内残余冲突由分配器
-- repair-bump 重试吸收（有界 3 次）。
-- next_seq 语义 = 下一个待分配的 seq（分配返回 next_seq-1 后行内存
-- next_seq+1 前的值）；BIGINT 对齐 Go int64。
-- 行生命周期：每会话至多一行，随会话生命周期同量级，无 sweeper。
-- 消费面藏在 ARANEA_SEQ_ALLOCATOR=off|pg（缺省 off 字节级旧行为，Go 进程
-- 内计数器）之后：off 档本表零写零读。仅建表：不加 ent schema、不播种。
-- 幂等，重跑安全。
CREATE TABLE IF NOT EXISTS session_seq_counters (
  session_id TEXT   PRIMARY KEY,
  next_seq   BIGINT NOT NULL
);
