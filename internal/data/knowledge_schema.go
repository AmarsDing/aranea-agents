package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	knowledgeEmbeddingIVFFlatIndex = "knowledge_chunks_embedding_idx"
	knowledgeEmbeddingHNSWIndex    = "knowledge_chunks_embedding_hnsw"
	// graphCardLegacyHNSWIndex G-33 卡片档旧单维 ANN 索引（G-39 仅用于 DROP 退役，
	// 替代为 per-dim partial HNSW，见 graphCardDimHNSWIndexSQL）。
	graphCardLegacyHNSWIndex = "knowledge_graph_embeddings_embedding_hnsw"
	defaultHNSWEfSearch      = 40
)

func knowledgeHNSWIndexSQL() string {
	return fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s
			ON knowledge_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)`, knowledgeEmbeddingHNSWIndex)
}

// chunkEmbeddingDimHNSWIndexSQL G-39 侧表 per-dim partial HNSW（ADR-KN-20261021-G）：
// pgvector HNSW opclass 要求列带维度 typmod（裸型 vector 报 22023），故索引建
// 在表达式 `(embedding::vector(N))` 上（官方 halfvec/量化索引同款模式）；partial
// WHERE `dim = N` 把索引切片限定在单维桶——桶内 cast 恒成功。CONCURRENTLY 在线
// 构建（不持 SHARE 锁，大语料首启不阻塞写入/就绪）；已存在健康索引零动作，
// INVALID 残留经 ensureIndexConcurrent 先 DROP 再重建。
func chunkEmbeddingDimHNSWIndexName(dim int) string {
	return fmt.Sprintf("chunk_embeddings_hnsw_d%d", dim)
}
func chunkEmbeddingDimHNSWIndexSQL(dim int) string {
	return fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s
			ON chunk_embeddings USING hnsw ((embedding::vector(%d)) vector_cosine_ops) WITH (m = 16, ef_construction = 64)
			WHERE dim = %d`, chunkEmbeddingDimHNSWIndexName(dim), dim, dim)
}

// graphCardDimHNSWIndexSQL G-33 卡片档 per-dim partial HNSW（与侧表同约）。
func graphCardDimHNSWIndexName(dim int) string {
	return fmt.Sprintf("knowledge_graph_embeddings_hnsw_d%d", dim)
}
func graphCardDimHNSWIndexSQL(dim int) string {
	return fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s
			ON knowledge_graph_embeddings USING hnsw ((embedding::vector(%d)) vector_cosine_ops) WITH (m = 16, ef_construction = 64)
			WHERE dim = %d`, graphCardDimHNSWIndexName(dim), dim, dim)
}

// chunkEmbeddingArchiveDimHNSWIndexSQL G-32 向量档案档 per-dim partial HNSW
// （ADR-KN-20261022-I 决策 12④；与侧表/卡片档同约——表达式 cast + partial
// WHERE dim = N，CONCURRENTLY 在线构建由 ensureIndexConcurrent 兜底）。
func chunkEmbeddingArchiveDimHNSWIndexName(dim int) string {
	return fmt.Sprintf("chunk_embeddings_archive_hnsw_d%d", dim)
}
func chunkEmbeddingArchiveDimHNSWIndexSQL(dim int) string {
	return fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s
			ON chunk_embeddings_archive USING hnsw ((embedding::vector(%d)) vector_cosine_ops) WITH (m = 16, ef_construction = 64)
			WHERE dim = %d`, chunkEmbeddingArchiveDimHNSWIndexName(dim), dim, dim)
}

// ensureKnowledgeEmbeddingHNSW drops the legacy IVFFlat ANN index and creates
// HNSW. Idempotent: DROP IF EXISTS + CREATE IF NOT EXISTS.
func ensureKnowledgeEmbeddingHNSW(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, "DROP INDEX IF EXISTS "+knowledgeEmbeddingIVFFlatIndex); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, knowledgeHNSWIndexSQL())
	return err
}

func hnswEfSearch() int {
	n := defaultHNSWEfSearch
	if v := os.Getenv("KRATOS_KNOWLEDGE_HNSW_EF_SEARCH"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	if n < 10 {
		n = 10
	}
	if n > 400 {
		n = 400
	}
	return n
}

// EnsureKnowledgeSchema creates the knowledge tables and indexes when they do not exist.
// lg 可选（启动脚本式函数的务实妥协：测试零值 Noop，生产传 Gateway 记录对账事件）。
func EnsureKnowledgeSchema(ctx context.Context, db *sql.DB, dim int, lg ...loggateway.Logger) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`CREATE TABLE IF NOT EXISTS knowledge_collections (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			description     TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL,
			dim             INT  NOT NULL DEFAULT 1536,
			status          TEXT NOT NULL DEFAULT 'active',
			document_count  INT  NOT NULL DEFAULT 0,
			chunk_count     INT  NOT NULL DEFAULT 0,
			workspace       TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_documents (
			id            TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			source        TEXT NOT NULL,
			mime_type     TEXT NOT NULL DEFAULT '',
			size_bytes    BIGINT NOT NULL DEFAULT 0,
			chunk_count   INT    NOT NULL DEFAULT 0,
			status        TEXT   NOT NULL DEFAULT 'pending',
			error_message TEXT   NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_text TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS organized BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS asset_uri TEXT NOT NULL DEFAULT ''`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS knowledge_chunks (
			id            TEXT PRIMARY KEY,
			doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			collection_id TEXT NOT NULL,
			content       TEXT NOT NULL,
			embedding     vector(%d),
			metadata      JSONB NOT NULL DEFAULT '{}',
			chunk_index   INT   NOT NULL DEFAULT 0,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, dim),
		`DROP INDEX IF EXISTS knowledge_chunks_embedding_idx`,
		knowledgeHNSWIndexSQL(),
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_collection_idx
			ON knowledge_chunks(collection_id)`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_content_tsvector_idx
			ON knowledge_chunks USING GIN (to_tsvector('simple', content))`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_content_trgm_idx
			ON knowledge_chunks USING GIN (content gin_trgm_ops)`,

		// --- G-39 多维度向量共存侧表（ADR-KN-20261021-G；fresh 形态，存量库由迁移
		// 20261290 建裸表——迁移轨道免 pgvector 依赖，下方 ALTER 双轨补列） ---
		// PK (chunk_id, dim)：同一 chunk 可并存多代模型向量；换模型不换表、不
		// ALTER 主表、不作废旧向量（08-10 事故根因随 reconcileEmbeddingDim
		// 退役终结）。doc_id 冗余落列支撑按文档批量操作（MoveDocument 等）。
		`CREATE TABLE IF NOT EXISTS chunk_embeddings (
			chunk_id      TEXT NOT NULL REFERENCES knowledge_chunks(id) ON DELETE CASCADE,
			dim           INT  NOT NULL CHECK (dim > 0),
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			doc_id        TEXT NOT NULL,
			model         TEXT NOT NULL DEFAULT '',
			embedding     vector,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (chunk_id, dim)
		)`,
		`ALTER TABLE chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector`,
		`CREATE INDEX IF NOT EXISTS chunk_embeddings_collection_dim_idx
			ON chunk_embeddings (collection_id, dim)`,
		`CREATE INDEX IF NOT EXISTS chunk_embeddings_doc_idx
			ON chunk_embeddings (doc_id)`,

		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS root_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS sync_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_collections_root_path_key
			ON knowledge_collections (root_path) WHERE root_path <> ''`,

		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS rel_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS summary_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS tags JSONB`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS doc_type TEXT NOT NULL DEFAULT ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_documents_rel_path_key
			ON knowledge_documents (collection_id, rel_path) WHERE rel_path <> ''`,

		`CREATE TABLE IF NOT EXISTS knowledge_links (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			target_doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			link_type     TEXT NOT NULL,
			context       TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_links_doc_idx ON knowledge_links(doc_id)`,
		`CREATE INDEX IF NOT EXISTS knowledge_links_target_idx ON knowledge_links(target_doc_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_links_unique
			ON knowledge_links(doc_id, target_doc_id, link_type)`,

		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS vault_backend TEXT NOT NULL DEFAULT 'local'`,

		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_fail_count INT NOT NULL DEFAULT 0`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_last_tried TIMESTAMPTZ`,
		`CREATE INDEX IF NOT EXISTS knowledge_documents_embed_degraded_idx
			ON knowledge_documents (collection_id, embed_last_tried) WHERE embed_fail_count > 0`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'collection'`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS owner_user_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS knowledge_documents_visibility_idx
			ON knowledge_documents (collection_id, visibility, owner_user_id)`,
		// --- G-08 部门级可见性（fresh 形态；存量库由迁移 20261284 补列） ---
		// shared_with：visibility='shared' 时的 org 节点 ID 名单（部门/岗位），
		// TEXT[] + GIN 支撑谓词 `shared_with && $n`（ADR-KN-20261020-E）。
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS shared_with TEXT[] NOT NULL DEFAULT '{}'`,
		`CREATE INDEX IF NOT EXISTS knowledge_documents_shared_with_gin
			ON knowledge_documents USING GIN (shared_with)`,
		`CREATE INDEX IF NOT EXISTS knowledge_documents_collection_hash_idx
			ON knowledge_documents (collection_id, content_hash) WHERE content_hash <> ''`,

		// --- G-33 图谱卡片向量档（ADR-KN-20261020-H + 20261021-G dim 裸型化；
		// fresh 形态，存量库由迁移 20261286 建表、20261290 补 dim——迁移轨道免
		// pgvector 依赖，下方 ALTER 双轨补齐） ---
		// ref_id 无 FK：kind 复用实体/链接两宿主表无法单一 FK，孤儿行由
		// GraphProjectionWorker 墓碑回收；可见性不落列，读路径经宿主文档投影
		// 下推 G-08 中央谓词。embedding 裸型 vector：多维度卡片共存（G-39 同轮），
		// per-dim partial HNSW 由 ensureChunkEmbeddingDimIndexes 注册。
		`CREATE TABLE IF NOT EXISTS knowledge_graph_embeddings (
			kind          TEXT NOT NULL CHECK (kind IN ('entity','edge')),
			ref_id        BIGINT NOT NULL,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			text          TEXT NOT NULL,
			source_hash   TEXT NOT NULL DEFAULT '',
			dim           INT  NOT NULL DEFAULT 0,
			embedding     vector,
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (kind, ref_id)
		)`,
		// 存量库（迁移 20261286 已建表但无 vector 列）补列；fresh 库 CREATE 已含，
		// ADD COLUMN IF NOT EXISTS 幂等。裸型化（typmod 剥离）与旧单维 HNSW DROP
		// 在 ensureMultiDimEmbeddingLayer 尾部完成。
		`ALTER TABLE knowledge_graph_embeddings ADD COLUMN IF NOT EXISTS embedding vector`,
		`ALTER TABLE knowledge_graph_embeddings ADD COLUMN IF NOT EXISTS dim INT NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS knowledge_graph_embeddings_collection_kind_idx
			ON knowledge_graph_embeddings (collection_id, kind)`,

		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS cited_count INT NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS knowledge_chunk_citations (
			chunk_id  TEXT NOT NULL,
			turn_id   TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (chunk_id, turn_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunk_citations_turn
			ON knowledge_chunk_citations(turn_id)`,
		// --- 父子分块（G-06，2026-10-20；fresh 形态，存量库由迁移 20261283 补列） ---
		// parent_chunk_id：子块→父块行 ID（空串 = 独立块/父块自身；不加自引用 FK，
		// 应用层单事务维护）；chunk_role：child|parent，父块仅语境回填不进召回。
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS parent_chunk_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS chunk_role TEXT NOT NULL DEFAULT 'child'`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_parent_idx
			ON knowledge_chunks (parent_chunk_id) WHERE parent_chunk_id <> ''`,
		// 摄取生效的分块三元组落库（reembed 保形）。
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_strategy TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_size INT NOT NULL DEFAULT 0`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_overlap INT NOT NULL DEFAULT 0`,
		// --- SP1-C 跨库双链解析（fresh 形态；存量库由迁移 20261204 补列） ---
		// documents.title/aliases：Resolver 文档键（frontmatter 物化）；
		// links.weight：N-3 投影权重（同文档对块边数聚合）。
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS aliases JSONB`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS weight INT NOT NULL DEFAULT 1`,

		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS relation TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS weight_f DOUBLE PRECISION NOT NULL DEFAULT 1.0`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS valid_to TIMESTAMPTZ`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`DROP INDEX IF EXISTS knowledge_links_unique`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_links_unique
			ON knowledge_links (doc_id, target_doc_id, link_type, relation)
			WHERE valid_to IS NULL`,
		`CREATE INDEX IF NOT EXISTS knowledge_links_valid_idx
			ON knowledge_links USING GIST (tstzrange(valid_from, COALESCE(valid_to, 'infinity')))`,
		`CREATE INDEX IF NOT EXISTS knowledge_links_active_idx
			ON knowledge_links (collection_id, doc_id) WHERE valid_to IS NULL`,
		// G-05-asof R 轴读索引（ADR-KN-20261020-J；fresh 形态，存量库由迁移 20261287 补）：
		// R-only/双轴谓词 recorded_at <= $R 驱动范围，collection_id 前置等值——
		// 无它则全库 as-of 图读退化全表 seq scan（历史行只增不删）。
		`CREATE INDEX IF NOT EXISTS knowledge_links_recorded_idx
			ON knowledge_links (collection_id, recorded_at)`,
		`CREATE TABLE IF NOT EXISTS knowledge_access_log (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			doc_id TEXT NOT NULL,
			accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			query_hash TEXT,
			session_id TEXT,
			user_id TEXT,
			agent_id TEXT,
			"rank" INTEGER NOT NULL DEFAULT 0,
			score DOUBLE PRECISION NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_access_log_doc_idx
			ON knowledge_access_log (collection_id, doc_id, accessed_at DESC)`,
		// G-15 审计足迹：按用户的审计/足迹查询索引（20261282 同型，存量库走迁移）。
		`CREATE INDEX IF NOT EXISTS knowledge_access_log_user_idx
			ON knowledge_access_log (user_id, accessed_at DESC)`,

		// --- 实体治理（G5-F B9/B12）：name_norm 承载唯一性（展示名 name 保留首见写法）；
		// 存量库由迁移 20261129 回填/合并后建唯一索引，此处定义 fresh 库最终形态 ---
		`CREATE TABLE IF NOT EXISTS knowledge_entities (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			name          TEXT NOT NULL,
			entity_type   TEXT NOT NULL DEFAULT '',
			name_norm     TEXT NOT NULL DEFAULT '',
			CONSTRAINT knowledge_entities_name_norm_key UNIQUE (collection_id, name_norm)
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_entity_aliases (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			entity_id     BIGINT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
			alias_norm    TEXT NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (collection_id, alias_norm)
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_entity_aliases_entity_idx
			ON knowledge_entity_aliases(entity_id)`,
		`CREATE TABLE IF NOT EXISTS knowledge_doc_entities (
			collection_id TEXT NOT NULL,
			doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			entity_id     BIGINT NOT NULL REFERENCES knowledge_entities(id) ON DELETE CASCADE,
			mentions      INT NOT NULL DEFAULT 1,
			PRIMARY KEY (doc_id, entity_id)
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_doc_entities_collection_idx
			ON knowledge_doc_entities(collection_id, entity_id)`,

		`CREATE TABLE IF NOT EXISTS knowledge_relation_vocab (
			relation    TEXT PRIMARY KEY,
			tier        TEXT NOT NULL,
			proposed_by TEXT,
			use_count   INT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO knowledge_relation_vocab (relation, tier, proposed_by) VALUES
			('is-a', 'core', 'system'),
			('part-of', 'core', 'system'),
			('depends-on', 'core', 'system'),
			('causes', 'core', 'system'),
			('applies-to', 'core', 'system'),
			('contradicts', 'core', 'system'),
			('supersedes', 'core', 'system'),
			('evolves-from', 'core', 'system')
		ON CONFLICT (relation) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS knowledge_relation_state (
			doc_id                 TEXT PRIMARY KEY REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			collection_id          TEXT NOT NULL,
			content_hash           TEXT NOT NULL DEFAULT '',
			entities_extracted_at  TIMESTAMPTZ,
			relations_extracted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_relation_state_collection_idx
			ON knowledge_relation_state (collection_id)`,

		`CREATE TABLE IF NOT EXISTS knowledge_fact_version (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			doc_id        TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			fact_id       TEXT,
			old_body      TEXT NOT NULL,
			new_body      TEXT,
			superseded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_fact_version_doc_idx
			ON knowledge_fact_version (doc_id, superseded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS knowledge_fact_version_collection_idx
			ON knowledge_fact_version (collection_id, superseded_at DESC)`,
		`CREATE TABLE IF NOT EXISTS knowledge_governance_proposal (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			kind          TEXT NOT NULL,
			payload       JSONB NOT NULL,
			risk          TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'pending',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at   TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS knowledge_governance_proposal_status_idx
			ON knowledge_governance_proposal (collection_id, status, risk)`,

		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS stale_at TIMESTAMPTZ`,

		// --- G-32 内容版本链+增量重建基座（ADR-KN-20261022-I，2026-10-22 拍板；
		// fresh 形态，存量库由迁移 20261294 建三表/列/视图+种子——迁移轨道免
		// pgvector 依赖，向量档案 embedding 列与 per-dim partial HNSW 走
		// ALTER/ensure 双轨；种子纯 SQL 仅迁移内一次性，Ensure 不挂种子循环）。
		// 置 stmts 末位：化身视图与前表引用 cited_count/parent_chunk_id/chunk_role
		// 等先行列 ---
		// document_versions：轻版本账本，PK (doc_id, version_no)；change_kind 值空间
		// 一次入列（seed|create|edit|rename|supersede|tombstone，producer 渐进）。
		// 不设 FK——物理删文档不得级联抹账：tombstone 行与归档化身区间的
		// valid_to 连接点必须存活（决策12「tombstone 可回放」），清理由
		// TK-G32-03 GC 挂账负责（与 chunk_archive 无 FK 同源设计）。
		`CREATE TABLE IF NOT EXISTS document_versions (
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
		)`,
		// chunk_archive：删块同事务物化归档，无 FK（cited_count 快照随行；
		// 「授权用当下、内容用历史」，GC 挂 TK-G32-03）。content_key=稳定键
		//（口径 'sha1:'+hex 裸字节，与 knowledge_chunks.content_key 同登记）。
		`CREATE TABLE IF NOT EXISTS chunk_archive (
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
		)`,
		`CREATE INDEX IF NOT EXISTS chunk_archive_chunk_idx
			ON chunk_archive (chunk_id)`,
		`CREATE INDEX IF NOT EXISTS chunk_archive_collection_idx
			ON chunk_archive (collection_id, archived_at)`,
		// chunk_embeddings_archive：死化身向量镜档（决策 12②，与 chunk_embeddings
		// 同型去 FK）；namespace 下死化身行永不 JOIN current 读面，零横切保
		// G-05 条款①。embedding 裸型列存量库由下行 ALTER 幂等补齐。
		`CREATE TABLE IF NOT EXISTS chunk_embeddings_archive (
			chunk_id      TEXT NOT NULL,
			dim           INT  NOT NULL CHECK (dim > 0),
			collection_id TEXT NOT NULL,
			doc_id        TEXT NOT NULL,
			model         TEXT NOT NULL DEFAULT '',
			embedding     vector,
			archived_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (chunk_id, dim)
		)`,
		`ALTER TABLE chunk_embeddings_archive ADD COLUMN IF NOT EXISTS embedding vector`,
		`CREATE INDEX IF NOT EXISTS chunk_embeddings_archive_collection_dim_idx
			ON chunk_embeddings_archive (collection_id, dim)`,
		// 稳定键列（决策 2）：配对权威恒 Go 侧现算，列仅寻址/审计/回填加速。
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS content_key TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_doc_content_key_idx
			ON knowledge_chunks (doc_id, content_key)`,
		// 化身区间视图（决策 12①）：as-of 时态读唯一区间推导口——出生版本由
		// 代际命名空间段解析，legacy 位置 ID 归 seed 版本档；死亡=归档
		// revision_no 版本 created_at；current 化身 valid_to='infinity'。
		// 归档腿 valid_to 对 valid_from GREATEST clamp（CON-13 双轨镜像——
		// 与迁移 20261294 视图文本逐字符一致：时钟回拨不产生负区间；
		// 同 tick [t,t) 坍缩根治挂 T4 写入侧钳位，偏差登记开发计划）。
		`CREATE OR REPLACE VIEW knowledge_chunk_incarnations AS
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
LEFT JOIN firstv fv ON fv.doc_id = a.doc_id`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("knowledge schema: %w", err)
		}
	}
	var logger loggateway.Logger
	if len(lg) > 0 {
		logger = lg[0]
	}

	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS knowledge_collections_workspace_name_uidx
		ON knowledge_collections (workspace, name)`); err != nil && logger != nil {
		logger.Warn("knowledge collections (workspace, name) unique index skipped",
			loggateway.StepID("data.schema.knowledge_ws_name_uidx"),
			loggateway.Err(err))
	}
	return ensureMultiDimEmbeddingLayer(ctx, db, dim, logger)
}

// ensureMultiDimEmbeddingLayer G-39 多维度向量共存层对账（ADR-KN-20261021-G，
// 取代 reconcileEmbeddingDim 的 08-10 事故 wipe 对账）：
//  1. chunks 存量向量按行 vector_dims() 真实维度回填进 chunk_embeddings
//     侧表（NOT EXISTS 反连接 + ON CONFLICT 幂等，崩溃后重启断点续补）；
//     维度取真值而非集合台账——台账与列 typmod 在历史 reconcile 后必然一致，
//     但行级 vector_dims() 是免证的；
//  2. 卡片档（G-33）dim 台账回填（NULL embedding 的卡保持 dim=0，读路径
//     `t.dim = vector_dims($n)` 谓词天然排除——无向量本就不可命中）；
//  3. 卡片档 embedding 裸型化（typmod 剥离）+ 旧单维 HNSW 退役（先 DROP 后
//     ALTER，避免 PG 沿索引重建连带旧切片）；
//  4. 集合台账 dim 对齐 embedder 当前输出（仅展示/账本用途，G-39 后不再决定
//     写入合法性——写入维度恒取 len(embedding)）；
//  5. per-dim partial HNSW 注册（见 ensureChunkEmbeddingDimIndexes）。
//
// legacy knowledge_chunks.embedding 列与其 HNSW 保留 ≥1 版本（瞬时回滚浸泡，
// DROP 见 TK-G39-01）；旧 dim 侧表行空间回收见 TK-G39-02。
func ensureMultiDimEmbeddingLayer(ctx context.Context, db *sql.DB, dim int, lg loggateway.Logger) error {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	stmts := []string{
		`UPDATE knowledge_graph_embeddings SET dim = vector_dims(embedding)
			WHERE dim = 0 AND embedding IS NOT NULL`,
		`DROP INDEX IF EXISTS ` + graphCardLegacyHNSWIndex,
		`ALTER TABLE knowledge_graph_embeddings ALTER COLUMN embedding TYPE vector`,
		`INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding)
			SELECT c.id, vector_dims(c.embedding), c.collection_id, c.doc_id, kc.embedding_model, c.embedding
			FROM knowledge_chunks c
			JOIN knowledge_collections kc ON kc.id = c.collection_id
			WHERE c.embedding IS NOT NULL
				AND NOT EXISTS (
					SELECT 1 FROM chunk_embeddings ce
					WHERE ce.chunk_id = c.id AND ce.dim = vector_dims(c.embedding)
				)
			ON CONFLICT (chunk_id, dim) DO NOTHING`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("knowledge multi-dim layer: %w", err)
		}
	}
	// 孤儿 chunks 巡检（F1-B3）：历史库若存在 collection_id 悬空行（现行
	// 代码路径不产生——FK CASCADE 全覆盖，仅上古/手工修库数据可能引入），
	// 回填 JOIN 会静默跳过它们，TK-G39-01 删 legacy 列后不可恢复——记 Warn
	// 留痕（不阻断启动；修复靠人工核对）。
	var orphaned int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM knowledge_chunks c
		WHERE c.embedding IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM knowledge_collections kc WHERE kc.id = c.collection_id)`).Scan(&orphaned); err == nil && orphaned > 0 {
		lg.Warn("chunks with dangling collection_id skipped by side-table backfill",
			loggateway.StepID("data.schema.knowledge_backfill_orphan"),
			loggateway.Int("chunks", orphaned))
	}
	if dim > 0 {
		res, err := db.ExecContext(ctx, `UPDATE knowledge_collections SET dim = $1, updated_at = NOW()
			WHERE embedding_model <> '' AND dim <> $1`, dim)
		if err != nil {
			return fmt.Errorf("knowledge multi-dim layer: collections dim ledger: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			lg.Warn("knowledge collections dim ledger aligned to embedder dim (G-39 ledger, no vector wipe)",
				loggateway.StepID("data.schema.knowledge_dim_ledger"),
				loggateway.Int("dim", dim),
				loggateway.Int("collections", int(n)))
		}
	}
	if err := ensureChunkEmbeddingDimIndexes(ctx, db, dim); err != nil {
		return err
	}
	// CON-15 回填加速器（G-32）：content_key 空串 谓词无索引支撑时 backfill
	// 每批次 seq scan 全表。partial index 只覆盖 content_key=空串 行，本体
	// 回填完成后体积趋零、维护代价趋零（决策 2：回填不构成配对阻塞，此索引
	// 纯属加速器）。与 dim 索引同走 CONCURRENTLY 注册轨（多副本竞态由
	// ensureIndexConcurrent 的复查-重试容忍）。
	return ensureIndexConcurrent(ctx, db,
		"knowledge_chunks_content_key_pending_idx",
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS knowledge_chunks_content_key_pending_idx
			ON knowledge_chunks (id) WHERE content_key = ''`)
}

// ensureChunkEmbeddingDimIndexes 为侧表/卡片档/向量档案档（G-32）注册 per-dim partial HNSW。
// 纯增量：存在且健康（indisvalid）的索引零动作；空表建 HNSW 瞬时完成，因此
// 把当前 embedder dim 一并预注册（首次插入后首个查询不走 seq scan）。dim 集合
// = 侧表现存 dims ∪ 向量档案 dims ∪ 卡片档 dims（>0）∪ embedder 当前 dim。存量旧 dim 索引永不
// DROP（回滚浸泡）；索引注册表收敛见 TK-G39-03。构建一律 CONCURRENTLY
// （F1-B2：首启对大语料表不持 SHARE 锁，就绪路径不被长构建阻塞；
// CONCURRENTLY 不能进事务块，Ensure 各语句本就独立 Exec）。
func ensureChunkEmbeddingDimIndexes(ctx context.Context, db *sql.DB, curDim int) error {
	rows, err := db.QueryContext(ctx, `
		SELECT dim FROM chunk_embeddings
		UNION
		SELECT dim FROM chunk_embeddings_archive
		UNION
		SELECT dim FROM knowledge_graph_embeddings WHERE dim > 0`)
	if err != nil {
		return fmt.Errorf("knowledge multi-dim layer: list dims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	dims := map[int]struct{}{}
	if curDim > 0 {
		dims[curDim] = struct{}{}
	}
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			return fmt.Errorf("knowledge multi-dim layer: scan dim: %w", err)
		}
		if d > 0 {
			dims[d] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("knowledge multi-dim layer: iterate dims: %w", err)
	}
	for d := range dims {
		if err := ensureIndexConcurrent(ctx, db, chunkEmbeddingDimHNSWIndexName(d), chunkEmbeddingDimHNSWIndexSQL(d)); err != nil {
			return fmt.Errorf("knowledge multi-dim layer: hnsw dim=%d: %w", d, err)
		}
		if err := ensureIndexConcurrent(ctx, db, graphCardDimHNSWIndexName(d), graphCardDimHNSWIndexSQL(d)); err != nil {
			return fmt.Errorf("knowledge multi-dim layer: card hnsw dim=%d: %w", d, err)
		}
		if err := ensureIndexConcurrent(ctx, db, chunkEmbeddingArchiveDimHNSWIndexName(d), chunkEmbeddingArchiveDimHNSWIndexSQL(d)); err != nil {
			return fmt.Errorf("knowledge multi-dim layer: archive hnsw dim=%d: %w", d, err)
		}
	}
	return nil
}

// ensureIndexConcurrent 单张索引的健康性注册：存在且 indisvalid → 零动作；
// 存在但 INVALID（历史上 CONCURRENTLY 构建中断的残留）→ 先 DROP 再以
// CONCURRENTLY 重建；不存在 → 直接 CONCURRENTLY 创建。
//
// 并发创建竞态容忍（对抗评审 CON-10）：IF NOT EXISTS 在 PG 对同名索引并发
// 创建是 TOCTOU（存在性检查早于堆创建），多副本同刻首启可 42P07 失败环。
// 创建报错后进入复查-重试：赢家副本完成构建后本副本复查到 indisvalid 即视为
// 成功返回 nil；约 3×300ms 窗口内仍不可见才上抛原始错误（单副本真实语法/
// 空间错误不因重试被吞——最多多等 ~900ms）。
func ensureIndexConcurrent(ctx context.Context, db *sql.DB, name, createSQL string) error {
	var valid bool
	err := db.QueryRowContext(ctx,
		`SELECT i.indisvalid FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
		 WHERE c.relkind = 'i' AND c.relname = $1`, name).Scan(&valid)
	switch {
	case err == nil && valid:
		return nil
	case err == nil:
		if _, derr := db.ExecContext(ctx, `DROP INDEX `+name); derr != nil {
			return fmt.Errorf("drop invalid index %s: %w", name, derr)
		}
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("probe index %s: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, createSQL); err == nil {
		return nil
	} else {
		createErr := err
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return fmt.Errorf("create index %s: %w", name, createErr)
			case <-time.After(300 * time.Millisecond):
			}
			valid = false
			perr := db.QueryRowContext(ctx,
				`SELECT i.indisvalid FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
			 	 WHERE c.relkind = 'i' AND c.relname = $1`, name).Scan(&valid)
			if perr == nil && valid {
				return nil // 并发赢家副本已建成——竞态失败已收敛
			}
		}
		return fmt.Errorf("create index %s: %w", name, createErr)
	}
}
