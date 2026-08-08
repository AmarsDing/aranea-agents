-- 20261204_knowledge_resolve: SP1-C 跨库双链解析支撑列。
-- 幂等：IF NOT EXISTS；重复应用安全。
-- 语义锚点：
--   - knowledge_documents.title/aliases：Resolver 文档键（设计 S3：rel_path 无扩展名/标题/别名），
--     由 blockparse.ParseDocMeta 从 frontmatter 提取后随块索引物化回写。
--   - knowledge_links.weight：N-3 投影权重——同 (src_doc, dst_doc) 多条块边聚合为一条
--     explicit 文档边时 weight = 块边数（G5 图谱 size ∝ 被引数）。
--   - 存量行 weight 默认 1 与迁移前「一条边一票」语义一致，无需回填。

ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS aliases JSONB;

ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS weight INT NOT NULL DEFAULT 1;
