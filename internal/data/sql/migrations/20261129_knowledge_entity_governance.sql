-- 20261129 knowledge_entity_governance（G5-F B9/B12）
-- 存量库形态升级：knowledge_entities 加 name_norm 归一化列、废 name 唯一约束
-- （归一化后由 name_norm 承载唯一性）、新建 knowledge_entity_aliases 别名表。
-- name_norm 回填（PG 无 NFKC，须 Go 侧计算）与 (collection_id, name_norm) 唯一
-- 索引由迁移 Func 在回填 + 冲突组合并后执行；本文件仅做幂等结构补丁。

ALTER TABLE knowledge_entities ADD COLUMN IF NOT EXISTS name_norm TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_entities DROP CONSTRAINT IF EXISTS knowledge_entities_collection_id_name_key;

CREATE TABLE IF NOT EXISTS knowledge_entity_aliases (
    id            BIGSERIAL PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    entity_id     BIGINT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
    alias_norm    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (collection_id, alias_norm)
);
CREATE INDEX IF NOT EXISTS knowledge_entity_aliases_entity_idx
    ON knowledge_entity_aliases(entity_id);
