-- 20261333 office_events_plan_log（工作台 › 日程 P1，设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/08-日程.md §4）
--   日程从「事件」扩成「计划 + 工作内容」两本账：
--   kind     plan（打算做的，默认）| log（已经做了的工作记录）
--   status   todo（未开始，默认）| doing | done | cancelled ——「逾期」不存，按结束时间算
--   all_day  全天（出差、截止日）；全天和 log 都不参与时间重叠检测
--   outcome  完成情况 / 结果
-- 存量行按默认值即为「未开始的计划」，与原语义一致。幂等 IF NOT EXISTS，重跑安全。

ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "kind"    TEXT    NOT NULL DEFAULT 'plan';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "status"  TEXT    NOT NULL DEFAULT 'todo';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "all_day" BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "outcome" TEXT    NOT NULL DEFAULT '';
