-- 20261339 office_events_more（工作台 › 日程：对照 Google / Outlook / 飞书日历补齐）
-- 地点、会议链接、分类色；重复规则（RRULE 子集，见 internal/biz/office/recur.go）与跳过的日期。
-- 重复系列 reminded_at 记「已提醒那次的开始时间」。幂等。
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "location" TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "url"      TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "color"    TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "rrule"    TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "exdates"  TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_office_events_rrule ON office_events ("user_id", "start_at") WHERE "rrule" <> '';
