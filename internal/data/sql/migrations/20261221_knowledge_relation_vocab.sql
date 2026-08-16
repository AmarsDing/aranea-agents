-- Version 20261221: knowledge_relation_vocab + knowledge_relation_state (self-governing graph M2).
-- L2 语义关系层地基：
--   knowledge_relation_vocab  受控涌现谓词词表（core 硬编码闭环 / candidate LLM 提议 / promoted 治理提升）；
--   knowledge_relation_state  关系/实体抽取幂等状态（content_hash 一致即跳过，控 LLM 成本）。
-- Idempotent: IF NOT EXISTS / ON CONFLICT guards; safe to re-apply.

-- ── 谓词词表（受控涌现） ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS knowledge_relation_vocab (
  relation    TEXT PRIMARY KEY,
  tier        TEXT NOT NULL,           -- core / candidate / promoted
  proposed_by TEXT,                    -- system / llm / governance
  use_count   INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 核心谓词闭集（tier=core，设计 §3.4 硬编码闭环）。
-- co_activated 是统计边 link_type（无谓词语义），不在此表。
INSERT INTO knowledge_relation_vocab (relation, tier, proposed_by) VALUES
  ('is-a',        'core', 'system'),
  ('part-of',     'core', 'system'),
  ('depends-on',  'core', 'system'),
  ('causes',      'core', 'system'),
  ('applies-to',  'core', 'system'),
  ('contradicts', 'core', 'system'),
  ('supersedes',  'core', 'system'),
  ('evolves-from','core', 'system')
ON CONFLICT (relation) DO NOTHING;

-- ── 抽取幂等状态 ────────────────────────────────────────────────────────────
-- 按 content_hash 判重：文档内容未变不重复抽（LLM 成本闸门）；
-- 内容与 hash 均变才重抽。派生状态，可随时清空重建（重抽全量热文档）。
CREATE TABLE IF NOT EXISTS knowledge_relation_state (
  doc_id                 TEXT PRIMARY KEY REFERENCES knowledge_documents(id) ON DELETE CASCADE,
  collection_id          TEXT NOT NULL,
  content_hash           TEXT NOT NULL DEFAULT '',
  entities_extracted_at  TIMESTAMPTZ,
  relations_extracted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS knowledge_relation_state_collection_idx
  ON knowledge_relation_state (collection_id);
