-- Version 20261330: GitHub 研发情报面板（E 阶段 W2c G，需求文档 §4，拍板 G-1/G-2）。
--
-- 四表：
--   github_repos             watch 清单（github.manage 管理）+ 健康快照列随行
--                            （ci_status/open_prs/oldest_pr_age_hours/open_issues/
--                            health_updated_at——GetRepoHealth 读此；stale 由读侧
--                            按 health_updated_at 超窗裁决，诚实标记不落库）；
--                            credential_id 引用凭据模块条目，token 不出库
--                            （G-2：poller 经 RevealSecretForExecution 进程内取用）。
--   github_events            webhook 事件落库（G-1：ceoboard 通道 source=github
--                            分发至此）；delivery_id=X-GitHub-Delivery GUID
--                            全局唯一 → 幂等键（同 delivery 重投恰一条）；
--                            processed 标记消费状态，payload_json 原文留痕。
--   github_sla_snapshots     PR/issue 生命周期记录（一对象一行，
--                            UNIQUE(repo,kind,number) 幂等 upsert）；
--                            GetSlaReport 按窗口聚合 p50/p90。
--   github_trending_reports  热榜结构化分析（每日 job 产出；
--                            knowledge_doc_id 回链知识库 L1 文档）。
--
-- 幂等：IF NOT EXISTS，重跑安全。pg 为唯一支持驱动，时间列统一 TIMESTAMPTZ。

CREATE TABLE IF NOT EXISTS github_repos (
  "id"                  TEXT PRIMARY KEY,
  "full_name"           TEXT NOT NULL,                -- owner/repo（biz 校验格式）
  "credential_id"       TEXT NOT NULL,                -- 凭据模块条目 id（token 不出库）
  "webhook_ok"          BOOLEAN NOT NULL DEFAULT FALSE, -- 最近一次 webhook 事件落库成功
  "last_event_at"       TIMESTAMPTZ,                  -- 最近一次事件落库时间
  -- 健康快照（webhook/poller 更新；GetRepoHealth 读侧按 health_updated_at 裁 stale）
  "ci_status"           TEXT NOT NULL DEFAULT 'unknown', -- green | red | unknown
  "open_prs"            INTEGER NOT NULL DEFAULT 0,
  "oldest_pr_age_hours" DOUBLE PRECISION NOT NULL DEFAULT 0,
  "open_issues"         INTEGER NOT NULL DEFAULT 0,
  "health_updated_at"   TIMESTAMPTZ,                  -- 快照时间；NULL=从未采集
  "created_at"          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS github_repos_full_name ON github_repos ("full_name");

CREATE TABLE IF NOT EXISTS github_events (
  "id"             TEXT PRIMARY KEY,
  "delivery_id"    TEXT NOT NULL,                   -- X-GitHub-Delivery（幂等键）
  "repo_full_name" TEXT NOT NULL DEFAULT '',
  "event_type"     TEXT NOT NULL DEFAULT '',        -- pull_request | issues | check_suite | ...
  "action"         TEXT NOT NULL DEFAULT '',        -- opened | closed | synchronize | ...
  "payload_json"   TEXT NOT NULL DEFAULT '',        -- 原文留痕（排查用；不含 secret）
  "processed"      BOOLEAN NOT NULL DEFAULT FALSE,  -- 健康/SLA 派生消费标记
  "received_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 幂等：同 delivery 重投恰一条（ON CONFLICT DO NOTHING 由 data 层承载）。
CREATE UNIQUE INDEX IF NOT EXISTS github_events_delivery ON github_events ("delivery_id");
CREATE INDEX IF NOT EXISTS idx_github_events_repo ON github_events ("repo_full_name", "received_at" DESC);
CREATE INDEX IF NOT EXISTS idx_github_events_unprocessed ON github_events ("received_at") WHERE NOT "processed";

CREATE TABLE IF NOT EXISTS github_sla_snapshots (
  "id"                TEXT PRIMARY KEY,
  "repo_full_name"    TEXT NOT NULL,
  "kind"              TEXT NOT NULL,                -- pr | issue
  "number"            INTEGER NOT NULL,             -- PR/issue 编号
  "opened_at"         TIMESTAMPTZ,
  "first_response_at" TIMESTAMPTZ,                  -- 首次非作者响应（pr_first_response 分子）
  "review_rounds"     INTEGER NOT NULL DEFAULT 0,   -- review 轮次（pr_review_rounds）
  "closed_at"         TIMESTAMPTZ,                  -- 关闭/合并时间（issue_close、pr 关闭时长）
  "updated_at"        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 一对象一行，幂等 upsert（poller/webhook 重跑安全）。
CREATE UNIQUE INDEX IF NOT EXISTS github_sla_snapshots_obj
  ON github_sla_snapshots ("repo_full_name", "kind", "number");
CREATE INDEX IF NOT EXISTS idx_github_sla_window ON github_sla_snapshots ("kind", "opened_at");

CREATE TABLE IF NOT EXISTS github_trending_reports (
  "id"               TEXT PRIMARY KEY,
  "report_date"      TIMESTAMPTZ NOT NULL,          -- 报告日（UTC 自然日零时）
  "summary"          TEXT NOT NULL DEFAULT '',      -- 结构化解读（为何热/相关性）
  "items_json"       TEXT NOT NULL DEFAULT '',      -- [{repo, language, stars_delta, why}]
  "knowledge_doc_id" TEXT NOT NULL DEFAULT '',      -- 知识库 L1 文档回链
  "created_at"       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_github_trending_date ON github_trending_reports ("report_date" DESC);
