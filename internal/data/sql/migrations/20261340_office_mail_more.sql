-- 20261340 office_mail_more（工作台 › 邮件第二轮，对照 126 / Gmail 补齐）
-- 账号：签名、发出的信是否存到服务器「已发送」。
-- 邮件：标签（逗号分隔）、定时发送（只用于草稿）。按 Message-ID 去重用的索引。
-- 拒收名单：名单里的发件人来信直接进垃圾邮件。幂等。
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "signature" TEXT    NOT NULL DEFAULT '';
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "save_sent" BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "labels"  TEXT NOT NULL DEFAULT '';
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "send_at" TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_office_mail_messages_msgid
  ON office_mail_messages ("account_id", "message_id_hdr") WHERE "message_id_hdr" <> '';
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_send_at
  ON office_mail_messages ("send_at") WHERE "send_at" IS NOT NULL;

CREATE TABLE IF NOT EXISTS office_mail_blocked (
  "user_id"    TEXT NOT NULL,
  "address"    TEXT NOT NULL,            -- 小写
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY ("user_id", "address")
);
