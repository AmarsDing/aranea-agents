-- Version 20261222: knowledge_fact_version + knowledge_governance_proposal (self-governing graph M3).
-- L3 演化时序层地基：
--   knowledge_fact_version         supersedes 版本链——同 fact_id 再写入/仲裁 supersede 时
--                                  旧段快照留痕（演化可审计、可回滚），不污染 links 表；
--   knowledge_governance_proposal  治理提案（M3.2 矛盾仲裁 kind=conflict 高风险人工二审；
--                                  M4 dream_cycle 复用承载 stale/orphan/decay/merge 等提案）。
-- Idempotent: IF NOT EXISTS guards; safe to re-apply.

-- ── supersedes 版本链（旧段快照） ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS knowledge_fact_version (
  id            BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
  fact_id       TEXT,
  old_body      TEXT NOT NULL,          -- 被顶替的旧段整段（含 H2 标题行）
  new_body      TEXT,                   -- 新段整段
  superseded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS knowledge_fact_version_doc_idx
  ON knowledge_fact_version (doc_id, superseded_at DESC);
CREATE INDEX IF NOT EXISTS knowledge_fact_version_collection_idx
  ON knowledge_fact_version (collection_id, superseded_at DESC);

-- ── 治理提案（高风险人工二审；低风险 M4 起自动应用） ──────────────────────────
CREATE TABLE IF NOT EXISTS knowledge_governance_proposal (
  id            BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  kind          TEXT NOT NULL,        -- conflict/stale/orphan/decay/merge/moc_emerge/relation_promote/distill
  payload       JSONB NOT NULL,
  risk          TEXT NOT NULL,        -- low / high
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending / applied / rejected
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  resolved_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS knowledge_governance_proposal_status_idx
  ON knowledge_governance_proposal (collection_id, status, risk);
