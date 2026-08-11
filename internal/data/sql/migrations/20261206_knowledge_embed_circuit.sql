-- 20261206_knowledge_embed_circuit: SP2 #9 embedding 熔断（SiYuan block_embeddings
-- fail_count 同源设计，调研报告 docs/reports/2026-08-11-research-pkm-backend-opensource.md）。
-- 幂等：IF NOT EXISTS；重复应用安全。
-- 语义锚点：
--   - embed_fail_count：embedding 连续失败计数；0 = 熔断关闭（正常 embed）。
--     失败时 chunks 写 NULL 向量照常落库（词法索引不中断），后台按指数退避
--     （1min << (fc-1)，封顶 64min）重试补齐向量，成功复位 0。
--   - embed_last_tried：最近一次 embed 尝试时间（退避判定依据）；NULL = 从未失败。
--   - 部分索引承载 RetryDegradedEmbeddings 扫描（WHERE embed_fail_count > 0），
--     正常文档不进入索引，30s 轮询零成本。

ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_fail_count INT NOT NULL DEFAULT 0;
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_last_tried TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS knowledge_documents_embed_degraded_idx
    ON knowledge_documents (collection_id, embed_last_tried) WHERE embed_fail_count > 0;
