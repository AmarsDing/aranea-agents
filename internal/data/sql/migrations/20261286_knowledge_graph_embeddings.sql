-- Version 20261286: 图谱卡片向量档（G-33，ADR-KN-20261020-H）
-- knowledge_graph_embeddings：词条文（kind='entity'，每实体一卡）与谓词文
-- （kind='edge'，每 active semantic 边一卡）的独立向量档，注入 RRF 第三腿。
-- ref_id 不建 FK（kind 复用 kg_entities/knowledge_links 两宿主表，无法单一 FK）
-- ——孤儿行（实体归并硬删/边闭合）由 KnowledgeGraphProjectionWorker 墓碑回收。
-- 向量维度与 knowledge_chunks 同源由启动 reconcileEmbeddingDim 对账（2026-08-10
-- 事故型多表防线）。可见性不落列：查询经宿主文档投影下推 G-08 中央谓词。
-- Idempotent: "already exists" errors are treated as success by the migration runner (DB-N6).
CREATE TABLE IF NOT EXISTS knowledge_graph_embeddings (
  kind          TEXT NOT NULL CHECK (kind IN ('entity','edge')),
  ref_id        BIGINT NOT NULL,
  collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
  text          TEXT NOT NULL,
  source_hash   TEXT NOT NULL DEFAULT '',
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, ref_id)
);
CREATE INDEX IF NOT EXISTS knowledge_graph_embeddings_collection_kind_idx
  ON knowledge_graph_embeddings (collection_id, kind);
