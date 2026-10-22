-- Version 20261294: G-32 内容版本链+增量重建基座（ADR-KN-20261022-I，2026-10-22 拍板）
-- 三表 + 稳定键列 + 化身区间视图：
--
--   document_versions         轻版本账本。change_kind 值空间一次入列：
--                             seed|create|edit|rename|supersede|tombstone；version_no=
--                             同 tx COALESCE(MAX,0)+1（doc 行 FOR UPDATE 已串行化）。
--                             写入双条件：head 行缺失 OR head.content_hash≠正文 hash OR
--                             chunk 多重集变更【（added>0∧deleted>0）——∧ 限定词是决策 3
--                             原文，纯净增/纯净删不产行，防同 body 重嵌伪版本】
--                             （head 空串 = 未知档视为相等）。
--   chunk_archive             删块同事务物化归档。无 FK——cited_count 快照随行、
--                             「授权用当下、内容用历史」；集合级/区间闭合 GC 挂 TK-G32-03。
--   chunk_embeddings_archive  死化身向量镜档（与 20261290 chunk_embeddings 同型）。
--                             迁移轨道免 pgvector 依赖先例（20261286/20261290）：embedding
--                             裸型列与 per-dim partial HNSW 由 EnsureKnowledgeSchema
--                             双轨补齐。无 FK（GC 域）。
--   knowledge_chunks.content_key  稳定键列。口径登记 = HashContent（'sha1:'+hex 裸字节，
--                             vault_filer.go:684-688；非 blockparse sha256 bare、非
--                             relation-extract 裸 sha1）。列仅服务 SQL 寻址/审计/幂等
--                             回填——配对权威恒为 Go 侧按现载 content 现算；空串 行按现算
--                             键等价处理，回填未完成不构成配对阻塞。
--   knowledge_chunk_incarnations  化身区间视图（决策 12①）：代际命名空间段
--                             -v(\d+)-(ch|p)-\d+$ 携带出生版本号，legacy 位置派生 ID 归
--                             seed 版本档；死亡时刻=归档 revision_no 版本的 created_at；
--                             current 化身 valid_to='infinity'。归档腿 valid_to 对
--                             valid_from 做 GREATEST clamp——时钟回拨/同 tick 相邻版本
--                             不产生负区间（对抗评审 CON-13；同 tick 坍缩 [t,t) 的根治
--                             挂 T4 写入侧钳位，偏差已登记开发计划）。
--
-- 种子：文件尾部一次性纯 SQL（content_hash=空串 未知档 + asset_hash=documents.content_hash
-- + chunk_stats={chunk_count}，ON CONFLICT DO NOTHING）。Ensure 不挂种子循环（评审
-- graft：启动全表扫幂等空转不可接受）；content_key 与文本档版本行 body-hash 富化由
-- Go 启动回填 runner 幂等分批跑（sha1 无 SQL 先例，禁引 pgcrypto）。
--
-- 编号历程：初稿 20261292 → 当日 C3 连占 20261292/20261293（ee29fca85）→ 顺延至
-- 20261294（canonical 铁律：SQL 文件名/注册行/设计文档引用同改同 commit）。
-- Idempotent: "already exists"/"duplicate column" errors are treated as success (DB-N6).

-- document_versions 不设 FK：物理删除文档行不得级联抹掉版本台账——tombstone
-- 版本行与历史化身区间（knowledge_chunk_incarnations 归档腿的 valid_to 连接
-- 点）必须存活，决策12「tombstone 可回放而 current 查无」赖此。孤儿台账的
-- 物理清理由 TK-G32-03 GC 挂账负责。
CREATE TABLE IF NOT EXISTS document_versions (
  doc_id       TEXT NOT NULL,
  version_no   INT  NOT NULL CHECK (version_no > 0),
  content_hash TEXT NOT NULL DEFAULT '',
  asset_hash   TEXT NOT NULL DEFAULT '',
  change_kind  TEXT NOT NULL DEFAULT 'seed'
               CHECK (change_kind IN ('seed','create','edit','rename','supersede','tombstone')),
  chunk_stats  JSONB NOT NULL DEFAULT '{}',
  producer     TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (doc_id, version_no)
);

CREATE TABLE IF NOT EXISTS chunk_archive (
  doc_id          TEXT NOT NULL,
  chunk_id        TEXT NOT NULL,
  revision_no     INT  NOT NULL CHECK (revision_no > 0),
  collection_id   TEXT NOT NULL,
  content         TEXT NOT NULL,
  content_key     TEXT NOT NULL DEFAULT '',
  chunk_role      TEXT NOT NULL DEFAULT 'child',
  parent_chunk_id TEXT NOT NULL DEFAULT '',
  chunk_index     INT  NOT NULL DEFAULT 0,
  cited_count     INT  NOT NULL DEFAULT 0,
  metadata        JSONB NOT NULL DEFAULT '{}',
  archived_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (doc_id, chunk_id, revision_no)
);
CREATE INDEX IF NOT EXISTS chunk_archive_chunk_idx
  ON chunk_archive (chunk_id);
CREATE INDEX IF NOT EXISTS chunk_archive_collection_idx
  ON chunk_archive (collection_id, archived_at);

CREATE TABLE IF NOT EXISTS chunk_embeddings_archive (
  chunk_id      TEXT NOT NULL,
  dim           INT  NOT NULL CHECK (dim > 0),
  collection_id TEXT NOT NULL,
  doc_id        TEXT NOT NULL,
  model         TEXT NOT NULL DEFAULT '',
  archived_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (chunk_id, dim)
);
CREATE INDEX IF NOT EXISTS chunk_embeddings_archive_collection_dim_idx
  ON chunk_embeddings_archive (collection_id, dim);

ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS content_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS knowledge_chunks_doc_content_key_idx
  ON knowledge_chunks (doc_id, content_key);

CREATE OR REPLACE VIEW knowledge_chunk_incarnations AS
WITH vers AS (
  SELECT doc_id, version_no, created_at FROM document_versions
),
firstv AS (
  SELECT doc_id, MIN(created_at) AS t0 FROM vers GROUP BY doc_id
)
SELECT
  c.id AS chunk_id,
  c.doc_id,
  c.collection_id,
  c.content,
  c.content_key,
  c.chunk_role,
  c.parent_chunk_id,
  c.chunk_index,
  c.cited_count,
  c.metadata,
  COALESCE(vb.created_at, fv.t0, c.created_at) AS valid_from,
  'infinity'::timestamptz AS valid_to,
  FALSE AS archived,
  NULL::timestamptz AS archived_at
FROM knowledge_chunks c
LEFT JOIN vers vb ON vb.doc_id = c.doc_id
  AND vb.version_no = COALESCE(substring(c.id from '-v([0-9]+)-(?:ch|p)-[0-9]+$')::int, 1)
LEFT JOIN firstv fv ON fv.doc_id = c.doc_id
UNION ALL
SELECT
  a.chunk_id,
  a.doc_id,
  a.collection_id,
  a.content,
  a.content_key,
  a.chunk_role,
  a.parent_chunk_id,
  a.chunk_index,
  a.cited_count,
  a.metadata,
  COALESCE(vb.created_at, fv.t0, a.archived_at) AS valid_from,
  GREATEST(COALESCE(vd.created_at, a.archived_at),
           COALESCE(vb.created_at, fv.t0, a.archived_at)) AS valid_to,
  TRUE AS archived,
  a.archived_at
FROM chunk_archive a
LEFT JOIN vers vb ON vb.doc_id = a.doc_id
  AND vb.version_no = COALESCE(substring(a.chunk_id from '-v([0-9]+)-(?:ch|p)-[0-9]+$')::int, 1)
LEFT JOIN vers vd ON vd.doc_id = a.doc_id AND vd.version_no = a.revision_no
LEFT JOIN firstv fv ON fv.doc_id = a.doc_id;

-- CON-9 老库升级护栏：种子引用 d.content_hash（该列此前仅由
-- EnsureKnowledgeSchema 双轨创建，启动顺序=迁移循环先于 Ensure），从引入
-- content_hash 之前的构建直接升级时缺失列会让种子语句 entire 被静默跳过
-- 且迁移仍记为已应用（executeSQLFileWithDialect UndefinedObject 容忍 +
-- recordMigrationApplied），版本账本永久空。迁移内自带幂等 ALTER 与
-- Ensure 互为双轨（ADD COLUMN IF NOT EXISTS 天然幂等）。
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '';

INSERT INTO document_versions (doc_id, version_no, content_hash, asset_hash, change_kind, chunk_stats, producer, created_at)
SELECT d.id, 1, '', d.content_hash, 'seed',
       jsonb_build_object('chunk_count', d.chunk_count),
       'g32-seed',
       d.created_at
FROM knowledge_documents d
ON CONFLICT (doc_id, version_no) DO NOTHING;
