-- 20261273 turns_v2_seq_per_session（R4-Q3）：turns_v2.seq 从全局共享计数
-- （task/turn/step 混用同一 SeqAssigner，实测同会话 turn seq 呈 1→23→428）
-- 重排为 per-session 单调 1,2,3…：按 session_id 分区、started_at+id 定序。
-- member turn 在成员会话内独立编号（session_id = 成员会话 id）。
-- 幂等：重跑时 ranked.new_seq 与 seq 已一致，WHERE 子句不命中任何行。
-- 双方言兼容：PG 与 SQLite(>=3.25) 均支持窗口函数 + 相关子查询 UPDATE。
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY started_at, id) AS new_seq
  FROM turns_v2
)
UPDATE turns_v2
SET seq = (SELECT new_seq FROM ranked WHERE ranked.id = turns_v2.id)
WHERE EXISTS (
  SELECT 1 FROM ranked WHERE ranked.id = turns_v2.id AND ranked.new_seq <> turns_v2.seq
);
