-- Version 20261290: 多维度向量共存侧表（G-39，ADR-KN-20261021-G）
-- chunk_embeddings：chunk 向量按 dim 分列落侧表，PK (chunk_id, dim)——换模型
-- 不再 ALTER 主表 typmod、不再置 NULL 作废（08-10 事故核清路径随
-- reconcileEmbeddingDim 原子退役终结）。迁移轨道免 pgvector 依赖先例：
-- embedding 列（vector 裸型）与 per-dim partial HNSW 由 EnsureKnowledgeSchema
-- 双轨补齐 + backfill（JOIN collections 账本 dim，ON CONFLICT 幂等）。
-- knowledge_graph_embeddings（G-33 卡片档）同轮补 dim 列；其 embedding 裸型化
-- 与 per-dim 索引同样在 Ensure 轨道完成。
-- Idempotent: "already exists"/"duplicate column" errors are treated as success (DB-N6).
CREATE TABLE IF NOT EXISTS chunk_embeddings (
  chunk_id      TEXT NOT NULL REFERENCES knowledge_chunks(id) ON DELETE CASCADE,
  dim           INT  NOT NULL CHECK (dim > 0),
  collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
  doc_id        TEXT NOT NULL,
  model         TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (chunk_id, dim)
);
CREATE INDEX IF NOT EXISTS chunk_embeddings_collection_dim_idx
  ON chunk_embeddings (collection_id, dim);
CREATE INDEX IF NOT EXISTS chunk_embeddings_doc_idx
  ON chunk_embeddings (doc_id);
ALTER TABLE knowledge_graph_embeddings ADD COLUMN dim INT NOT NULL DEFAULT 0;
