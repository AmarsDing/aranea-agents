-- 20261334 office_mail_p1（工作台 › 邮件 P1，设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/09-邮件.md §4）
-- 账号：发件地址、服务商、SMTP、连接状态、首次同步范围。
-- 邮件：收件人 / 抄送、已读 / 星标 / 归档、文件夹（收件箱 / 已发送 / 草稿）、清洗后的 HTML、
--       Message-ID / References（回复串线用）。
-- uid 从此是 IMAP UID（不再是序号）；唯一索引只管收件箱里同步来的邮件——
-- 已发送 / 草稿的 uid 是 0，不能被它挡住。幂等：IF NOT EXISTS，重跑安全。

ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "email"       TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "provider"    TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "smtp_host"   TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "smtp_port"   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "smtp_tls"    TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "status"      TEXT    NOT NULL DEFAULT '';  -- '' 未检查 | ok | auth_failed | unreachable
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "last_error"  TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "sync_since"  TIMESTAMPTZ;                  -- 首次同步只取这天之后的；空 = 全部

ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "to_addrs"       TEXT NOT NULL DEFAULT '';
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "cc_addrs"       TEXT NOT NULL DEFAULT '';
-- 存量行按「已读」入列，之后新行默认未读
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "seen"           BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE office_mail_messages ALTER COLUMN "seen" SET DEFAULT FALSE;
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "flagged"        BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "archived_at"    TIMESTAMPTZ;
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "folder"         TEXT NOT NULL DEFAULT 'inbox'; -- inbox | sent | drafts
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "body_html"      TEXT NOT NULL DEFAULT '';      -- 入库时清洗过
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "message_id_hdr" TEXT NOT NULL DEFAULT '';
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "references_hdr" TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS office_mail_messages_acc_uid;
CREATE UNIQUE INDEX IF NOT EXISTS office_mail_messages_acc_uid_inbox
  ON office_mail_messages ("account_id", "uid") WHERE "account_id" <> '' AND "folder" = 'inbox';
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_folder
  ON office_mail_messages ("user_id", "folder", "received_at" DESC NULLS LAST);
