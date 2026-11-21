-- 20261338 office_mail_trash_spam（工作台 › 邮件：已删除 / 垃圾邮件，对照 126 / Gmail 补齐）
-- 邮件：trashed_at（删除先进「已删除」，可恢复；彻底删除才真删行）、spam（垃圾邮件分类）。
-- folder 多一个取值 junk：从服务器垃圾邮件文件夹同步来的信（UID 空间与收件箱不同，单独唯一索引）。
-- 账号：junk_last_uid（垃圾邮件文件夹的同步游标）。幂等。
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "trashed_at" TIMESTAMPTZ;
ALTER TABLE office_mail_messages ADD COLUMN IF NOT EXISTS "spam"       BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE office_mail_accounts ADD COLUMN IF NOT EXISTS "junk_last_uid" BIGINT NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS office_mail_messages_acc_uid_junk
  ON office_mail_messages ("account_id", "uid") WHERE "account_id" <> '' AND "folder" = 'junk';
