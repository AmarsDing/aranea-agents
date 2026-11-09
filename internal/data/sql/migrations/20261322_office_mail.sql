-- Version 20261322: office 邮件（E 阶段 W1d，需求文档 §1，拍板 E-1..E-5）。
--
-- 四表（自 workbench schema.sql L210-240 平移 + 拍板扩展）：
--   office_mail_accounts     邮箱账号（E-1：密码写后即弃、仅存 password_enc
--                            AES-GCM 信封密文，无明文列、无回显 RPC；
--                            tls_mode: tls|starttls|none 三模式平移）
--   office_mail_messages     邮件（部分唯一索引 (account_id,uid) WHERE
--                            account_id<>'' 幂等去重——.eml 导入行 account_id
--                            为空串不参与唯一约束，语义平移；classification
--                            密级列随行（D6 标签体系落位前先行留痕）；
--                            quadrant 四象限列预留（E-3，F2 分拣 job 回填）；
--                            task_ref 转任务回写）
--   office_mail_attachments  附件（E-4：BYTEA 入库 + 16MB 上限对齐 IMAP
--                            字面量上限；message_id 外键 ON DELETE CASCADE；
--                            knowledge_doc_id 入知识库回链）
--   office_tasks             个人任务（邮件转任务 F1 落点；统一任务中心落位
--                            前的轻量承载）
--
-- 每封邮件一个事务（邮件+附件同生共死，失败可整体重试）的语义由 biz 层
-- 组合仓储承载；本迁移只保证约束与索引。幂等：IF NOT EXISTS，重跑安全。
-- pg 为唯一支持驱动，时间列统一 TIMESTAMPTZ。

CREATE TABLE IF NOT EXISTS office_mail_accounts (
  "id"            TEXT PRIMARY KEY,
  "workspace_id"  TEXT NOT NULL DEFAULT '',
  "user_id"       TEXT NOT NULL,                  -- 账号属主（JWT 主体字符串化 id）
  "name"          TEXT NOT NULL DEFAULT '',       -- 展示名
  "host"          TEXT NOT NULL,
  "port"          INTEGER NOT NULL DEFAULT 993,
  "tls_mode"      TEXT NOT NULL DEFAULT 'tls',    -- tls | starttls | none
  "username"      TEXT NOT NULL,
  "password_enc"  TEXT NOT NULL,                  -- AES-GCM 信封密文（credential 设施）；无明文列
  "last_uid"      BIGINT NOT NULL DEFAULT 0,      -- 同步游标（IMAP UID 单调递增）
  "last_sync_at"  TIMESTAMPTZ,                    -- 从未同步留空
  "created_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_mail_accounts_user ON office_mail_accounts ("user_id");

CREATE TABLE IF NOT EXISTS office_mail_messages (
  "id"             TEXT PRIMARY KEY,
  "workspace_id"   TEXT NOT NULL DEFAULT '',
  "user_id"        TEXT NOT NULL,
  "account_id"     TEXT NOT NULL DEFAULT '',      -- .eml 导入为空串（不参与唯一索引）
  "uid"            BIGINT NOT NULL DEFAULT 0,     -- IMAP UID；导入为 0
  "subject"        TEXT NOT NULL DEFAULT '',
  "sender"         TEXT NOT NULL DEFAULT '',
  "body"           TEXT NOT NULL DEFAULT '',      -- text/plain 优先（parseMailRaw 语义）
  "snippet"        TEXT NOT NULL DEFAULT '',      -- 列表摘要（正文前 200 字符，写时物化）
  "received_at"    TIMESTAMPTZ,                   -- 邮件头原样时间（多格式兼容解析后归一）
  "classification" INTEGER NOT NULL DEFAULT 0,    -- 密级（0=未标；L1-L5 语义随 D6 标签体系）
  "quadrant"       TEXT NOT NULL DEFAULT '',      -- E-3 四象限预留（F2 分拣回填）
  "task_ref"       TEXT NOT NULL DEFAULT '',      -- 转任务回链（统一任务中心落位后解析）
  "created_at"     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 幂等去重（平移 workbench 语义）：同账号同 UID 唯一；导入行（account_id=''）豁免。
CREATE UNIQUE INDEX IF NOT EXISTS office_mail_messages_acc_uid
  ON office_mail_messages ("account_id", "uid") WHERE "account_id" <> '';
-- 列表聚合：本人邮件按收件时间倒序（阅读列表）；账号维度同步排查。
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_user ON office_mail_messages ("user_id", "received_at" DESC NULLS LAST, "created_at" DESC);
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_account ON office_mail_messages ("account_id", "uid");

CREATE TABLE IF NOT EXISTS office_mail_attachments (
  "id"                TEXT PRIMARY KEY,
  "message_id"        TEXT NOT NULL REFERENCES office_mail_messages("id") ON DELETE CASCADE,
  "filename"          TEXT NOT NULL DEFAULT '',
  "content_type"      TEXT NOT NULL DEFAULT '',
  "size"              BIGINT NOT NULL DEFAULT 0,
  "content"           BYTEA NOT NULL,             -- E-4：BYTEA 入库；16MB 上限在 biz 层卡口
  "knowledge_doc_id"  TEXT NOT NULL DEFAULT '',   -- 入知识库回链
  "created_at"        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_mail_attachments_msg ON office_mail_attachments ("message_id");

-- office_tasks 个人任务（邮件转任务的 F1 落点；统一任务中心落位前的
-- 轻量承载，届时可整体映射迁移）。契约修正（需求文档 §1.3）：space 维度
-- 由服务端从调用者上下文解析（workspace_id+user_id），前端不再传 space_id。
CREATE TABLE IF NOT EXISTS office_tasks (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "user_id"      TEXT NOT NULL,
  "title"        TEXT NOT NULL,
  "body"         TEXT NOT NULL DEFAULT '',
  "source"       TEXT NOT NULL DEFAULT 'manual',  -- manual | mail
  "source_ref"   TEXT NOT NULL DEFAULT '',        -- 邮件转任务：office_mail_messages.id
  "status"       TEXT NOT NULL DEFAULT 'open',    -- open | done
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_tasks_user ON office_tasks ("user_id", "status", "created_at" DESC);
