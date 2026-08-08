-- 20261203_knowledge_blocks: SP1-B 块级双链派生索引表（knowledge 域 Raw SQL，TEXT id 域内一致）。
-- 幂等：IF NOT EXISTS；重复应用安全。
-- 语义锚点：
--   - 派生索引纪律：blocks/refs 为可全量重放的派生数据，整文档删了重插，不做 diff。
--   - src_block_id ON DELETE CASCADE：块被重建删除时其出向边随之清除。
--   - dst_block_id / dst_doc_id ON DELETE SET NULL：目标消失时入向边转 dangling，raw_target 保留复活线索。
--   - 锚点库级部分唯一：UNIQUE(collection_id, anchor) WHERE anchor IS NOT NULL（未锚块不参与）。

CREATE TABLE IF NOT EXISTS knowledge_blocks (
    id            TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    ordinal       INT  NOT NULL,
    kind          TEXT NOT NULL,
    anchor        TEXT,
    heading_path  TEXT[],
    content_hash  TEXT NOT NULL DEFAULT '',
    text_excerpt  TEXT NOT NULL DEFAULT '',
    promoted_from TEXT,
    promoted_to   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_blocks_doc_ordinal_key
    ON knowledge_blocks (doc_id, ordinal);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_blocks_anchor_key
    ON knowledge_blocks (collection_id, anchor) WHERE anchor IS NOT NULL;

CREATE INDEX IF NOT EXISTS knowledge_blocks_collection_idx
    ON knowledge_blocks (collection_id);

CREATE TABLE IF NOT EXISTS knowledge_block_refs (
    id            BIGSERIAL PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    src_block_id  TEXT NOT NULL REFERENCES knowledge_blocks(id) ON DELETE CASCADE,
    dst_doc_id    TEXT REFERENCES knowledge_documents(id) ON DELETE SET NULL,
    dst_block_id  TEXT REFERENCES knowledge_blocks(id) ON DELETE SET NULL,
    raw_target    TEXT NOT NULL,
    alias         TEXT NOT NULL DEFAULT '',
    edge_type     TEXT NOT NULL,
    context       TEXT NOT NULL DEFAULT '',
    ambiguous     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS knowledge_block_refs_dst_block_idx
    ON knowledge_block_refs (dst_block_id);

CREATE INDEX IF NOT EXISTS knowledge_block_refs_dst_doc_idx
    ON knowledge_block_refs (dst_doc_id);

CREATE INDEX IF NOT EXISTS knowledge_block_refs_raw_target_idx
    ON knowledge_block_refs (collection_id, raw_target);

CREATE INDEX IF NOT EXISTS knowledge_block_refs_src_block_idx
    ON knowledge_block_refs (src_block_id);
