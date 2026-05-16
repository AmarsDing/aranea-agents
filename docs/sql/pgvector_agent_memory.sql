-- ============================================================
-- PostgreSQL pgvector: agent_memory 向量表
-- 不同 embedding 模型产生不同维度，每个维度对应一张分区表
-- 表名格式: agent_memory_{dim}
-- 使用前需先安装 pgvector 扩展: CREATE EXTENSION IF NOT EXISTS vector;
-- ============================================================

CREATE EXTENSION IF NOT EXISTS vector;

-- 示例: 1536 维 (OpenAI text-embedding-ada-002)
CREATE TABLE IF NOT EXISTS agent_memory_1536 (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  embedding vector(1536) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_1536_agent_uid ON agent_memory_1536 (agent_id, user_id);

-- 示例: 1024 维 (常见开源模型)
-- CREATE TABLE IF NOT EXISTS agent_memory_1024 (
--   id BIGSERIAL PRIMARY KEY,
--   agent_id TEXT NOT NULL,
--   user_id TEXT NOT NULL DEFAULT '',
--   content TEXT NOT NULL,
--   embedding vector(1024) NOT NULL,
--   created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
-- );
-- CREATE INDEX IF NOT EXISTS idx_agent_memory_1024_agent_uid ON agent_memory_1024 (agent_id, user_id);

-- 如需新增维度表，请参照以下模板:
-- CREATE TABLE IF NOT EXISTS agent_memory_{dim} (
--   id BIGSERIAL PRIMARY KEY,
--   agent_id TEXT NOT NULL,
--   user_id TEXT NOT NULL DEFAULT '',
--   content TEXT NOT NULL,
--   embedding vector({dim}) NOT NULL,
--   created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
-- );
-- CREATE INDEX IF NOT EXISTS idx_agent_memory_{dim}_agent_uid ON agent_memory_{dim} (agent_id, user_id);
