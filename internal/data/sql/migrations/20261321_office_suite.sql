-- Version 20261321: office 套件一期（E 阶段 W1c，需求文档 §2，拍板 K-1..K-5）。
--
-- 五表：
--   office_events           日程事件（自 workbench events 平移 + timezone/
--                           reminder_minutes/source 扩展；start/end 为原生
--                           TIMESTAMPTZ——范围聚合与 409 重叠检测依赖原生比较）
--   office_notifications    通知中心 MVP 收件箱（提醒/摘要/审批待办统一落表；
--                           写路径不开放 RPC，由生产方 job/服务进程内写入）
--   office_ical_tokens      ics 只读订阅令牌（token 只存 SHA-256 哈希，明文
--                           仅在创建响应出现一次；吊销即 feed 401）
--   office_apps             应用启动器 URL 快捷方式（K-5）
--   office_schedule_drafts  agent 排期草案（审批中心 kind=schedule_draft 的
--                           领域承载；批准时逐项复检 409 后事务落 events）
--
-- 幂等：IF NOT EXISTS / CREATE INDEX IF NOT EXISTS；不重写存量行，重跑安全。
-- pg 为唯一支持驱动（2026-10-21 起 SQLite 已移除），时间列统一 TIMESTAMPTZ。

CREATE TABLE IF NOT EXISTS office_events (
  "id"               TEXT PRIMARY KEY,
  "workspace_id"     TEXT NOT NULL DEFAULT '',
  "user_id"          TEXT NOT NULL,                 -- 事件属主（JWT 主体字符串化 id）
  "task_ref"         TEXT NOT NULL DEFAULT '',      -- 任务引用（统一任务中心落位后解析；可空）
  "task_title"       TEXT NOT NULL DEFAULT '',      -- 排期时快照（反查展示，替代 LEFT JOIN）
  "title"            TEXT NOT NULL,
  "note"             TEXT NOT NULL DEFAULT '',
  "start_at"         TIMESTAMPTZ NOT NULL,
  "end_at"           TIMESTAMPTZ NOT NULL,
  "timezone"         TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  "reminder_minutes" INTEGER NOT NULL DEFAULT 0,    -- 0=不提醒
  "source"           TEXT NOT NULL DEFAULT 'manual', -- manual | task_schedule | agent_draft
  "reminded_at"      TIMESTAMPTZ,                   -- 提醒已投递标记（幂等闸；NULL=未投递）
  "created_at"       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 范围聚合（K-1：月历一次请求）与重叠检测（同 user 时段相交）共用前导列。
CREATE INDEX IF NOT EXISTS idx_office_events_user_start ON office_events ("user_id", "start_at");
CREATE INDEX IF NOT EXISTS idx_office_events_task ON office_events ("task_ref") WHERE "task_ref" <> '';
-- 提醒扫描：未提醒 + 有提醒配置的事件按开始时间升序拾取。
CREATE INDEX IF NOT EXISTS idx_office_events_reminder ON office_events ("start_at")
  WHERE "reminder_minutes" > 0 AND "reminded_at" IS NULL;

CREATE TABLE IF NOT EXISTS office_notifications (
  "id"         TEXT PRIMARY KEY,
  "user_id"    TEXT NOT NULL,
  "kind"       TEXT NOT NULL,                       -- schedule_reminder | mail_digest | approval_todo | system
  "title"      TEXT NOT NULL,
  "body"       TEXT NOT NULL DEFAULT '',
  "link"       TEXT NOT NULL DEFAULT '',            -- 前端路由或外部 URL
  "read"       BOOLEAN NOT NULL DEFAULT FALSE,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_notifications_user ON office_notifications ("user_id", "read", "created_at" DESC);

CREATE TABLE IF NOT EXISTS office_ical_tokens (
  "id"         TEXT PRIMARY KEY,
  "user_id"    TEXT NOT NULL,
  "token_hash" TEXT NOT NULL UNIQUE,                -- SHA-256 hex；明文不落库
  "name"       TEXT NOT NULL DEFAULT '',
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "revoked_at" TIMESTAMPTZ                          -- NULL=有效
);
CREATE INDEX IF NOT EXISTS idx_office_ical_tokens_user ON office_ical_tokens ("user_id");

CREATE TABLE IF NOT EXISTS office_apps (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "name"         TEXT NOT NULL,
  "url"          TEXT NOT NULL,
  "icon"         TEXT NOT NULL DEFAULT '',
  "group_name"   TEXT NOT NULL DEFAULT '',
  "sort_order"   INTEGER NOT NULL DEFAULT 0,
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_apps_ws ON office_apps ("workspace_id", "group_name", "sort_order");

CREATE TABLE IF NOT EXISTS office_schedule_drafts (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "user_id"      TEXT NOT NULL,                     -- 草案落库后的日历属主（提交时 JWT 主体）
  "agent_id"     TEXT NOT NULL DEFAULT '',
  "session_id"   TEXT NOT NULL DEFAULT '',
  "status"       TEXT NOT NULL DEFAULT 'pending',   -- pending | approved | rejected
  "items_json"   TEXT NOT NULL,                     -- ScheduleDraftItem 数组 JSON
  "decided_by"   TEXT NOT NULL DEFAULT '',
  "decide_note"  TEXT NOT NULL DEFAULT '',
  "decided_at"   TIMESTAMPTZ,
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 审批中心 schedule_draft 源：按工作区扫 pending。
CREATE INDEX IF NOT EXISTS idx_office_schedule_drafts_pending ON office_schedule_drafts ("workspace_id", "created_at" DESC)
  WHERE "status" = 'pending';
