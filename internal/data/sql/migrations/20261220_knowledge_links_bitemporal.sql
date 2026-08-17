-- Version 20261220: knowledge_links bitemporal + knowledge_access_log (self-governing graph M1).
-- L3 时序地基：links 加双时态列（valid_from/valid_to 现实世界区间 + recorded_at 摄入时间）、
-- 语义谓词 relation、浮点权重 weight_f（Hebbian）、置信度 confidence。
-- L1 统计联想数据源：knowledge_access_log 记录每次检索命中（base-level 激活分 + Hebbian 共激活）。
-- Idempotent: IF NOT EXISTS guards; safe to re-apply.

-- ── knowledge_links 双时态 + 谓词 + 浮点权重 ────────────────────────────────
-- relation 定 NOT NULL DEFAULT ''（空串=未定型边）：普通四列唯一索引即可，
-- 避免 COALESCE 表达式索引导致既有 ON CONFLICT 列推断失败。
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS relation TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS weight_f DOUBLE PRECISION NOT NULL DEFAULT 1.0;
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0;
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS valid_to TIMESTAMPTZ;
-- 时态列先加可空形态：PG11+ 带 DEFAULT 的 ADD COLUMN 会直接填充存量行，
-- 导致 created_at 回填永不命中。先回填再收紧约束（幂等：重跑时 UPDATE 无 NULL 可命中）。
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ;
ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS recorded_at TIMESTAMPTZ;

-- 存量回填：valid_from / recorded_at 取 created_at（幂等，仅填 NULL）。
UPDATE knowledge_links SET valid_from = created_at WHERE valid_from IS NULL;
UPDATE knowledge_links SET recorded_at = created_at WHERE recorded_at IS NULL;

ALTER TABLE knowledge_links ALTER COLUMN valid_from SET DEFAULT NOW();
ALTER TABLE knowledge_links ALTER COLUMN valid_from SET NOT NULL;
ALTER TABLE knowledge_links ALTER COLUMN recorded_at SET DEFAULT NOW();
ALTER TABLE knowledge_links ALTER COLUMN recorded_at SET NOT NULL;

-- 唯一约束升级：只约束当前有效边；关闭行允许保留多个历史版本。
DROP INDEX IF EXISTS knowledge_links_unique;
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_links_unique
  ON knowledge_links (doc_id, target_doc_id, link_type, relation)
  WHERE valid_to IS NULL;

-- 时态 as-of 查询索引（tstzrange + GiST）。
CREATE INDEX IF NOT EXISTS knowledge_links_valid_idx
  ON knowledge_links USING GIST (tstzrange(valid_from, COALESCE(valid_to, 'infinity')));

-- 当前有效边热路径索引（扩散激活/图谱查询只读 valid_to IS NULL）。
CREATE INDEX IF NOT EXISTS knowledge_links_active_idx
  ON knowledge_links (collection_id, doc_id) WHERE valid_to IS NULL;

-- ── knowledge_access_log：检索命中日志 ─────────────────────────────────────
-- base-level 激活分（ln Σ access_t^-0.5）与 Hebbian 共激活（同 query_hash 两两强化）的数据源。
CREATE TABLE IF NOT EXISTS knowledge_access_log (
  id BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  doc_id TEXT NOT NULL,
  accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  query_hash TEXT,
  session_id TEXT
);
CREATE INDEX IF NOT EXISTS knowledge_access_log_doc_idx
  ON knowledge_access_log (collection_id, doc_id, accessed_at DESC);
