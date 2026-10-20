package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"aranea-agents/pkg/loggateway"
)

const (
	knowledgeEmbeddingIVFFlatIndex = "knowledge_chunks_embedding_idx"
	knowledgeEmbeddingHNSWIndex    = "knowledge_chunks_embedding_hnsw"
	defaultHNSWEfSearch            = 40
)

func knowledgeHNSWIndexSQL() string {
	return fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s
			ON knowledge_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)`, knowledgeEmbeddingHNSWIndex)
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
		// parent_chunk_id：子块→父块行 ID（'' = 独立块/父块自身；不加自引用 FK，
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
	return reconcileEmbeddingDim(ctx, db, dim, logger)
}

// reconcileEmbeddingDim 向量维度对账（2026-08-10 "expected N dimensions" 事故根修）：
// knowledge_chunks.embedding 的列类型在首次建表时固定为当时的 embedder 维度；
// embedder 换模型（维度变化）后 CREATE TABLE IF NOT EXISTS 不会修正列类型，
// 新维度向量插入全部被 PG 拒绝——而应用层按 knowledge_collections.dim 的校验
// 反而通过，错误只在 SQL 层爆炸，极难定位。
//
// 对账策略（幂等；维度一致时零动作）：
//  1. 旧维度向量对新 embedder 无检索意义 → 全部置 NULL 作废；
//  2. 受影响文档 content_hash 清空 + status 回 pending——vault 文档下轮 sync
//     自动重嵌入自愈；UI 上传文档（rel_path 空）无 sync 循环，Warn 日志提示
//     需人工重传（ReindexDocument 出口属能力缺口提案）；
//  3. 有语义层集合的 dim 快照同步为新维度——系统是单全局 embedder，集合 dim
//     只是创建时快照；向量已作废后不同步 = 应用层校验永远拒绝新插入（死库）。
//     embedding_model 名不动（Ensure 拿不到模型名，且仅作展示）；
//  4. ALTER 列类型 + 重建 HNSW 索引（先 DROP ivfflat/hnsw 再 ALTER，
//     避免 PG 级联重建沿用旧维度）。
func reconcileEmbeddingDim(ctx context.Context, db *sql.DB, dim int, lg loggateway.Logger) error {
	if dim <= 0 {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var cur int
	if err := db.QueryRowContext(ctx, `
		SELECT a.atttypmod FROM pg_attribute a
		WHERE a.attrelid = 'knowledge_chunks'::regclass AND a.attname = 'embedding'`).Scan(&cur); err != nil {
		return fmt.Errorf("knowledge dim reconcile: read embedding typmod: %w", err)
	}
	if cur == dim {
		return nil
	}
	stmts := []string{
		`DROP INDEX IF EXISTS knowledge_chunks_embedding_idx`,
		`DROP INDEX IF EXISTS knowledge_chunks_embedding_hnsw`,
		`UPDATE knowledge_chunks SET embedding = NULL WHERE embedding IS NOT NULL`,
		`UPDATE knowledge_documents SET content_hash = '', status = 'pending', updated_at = NOW()
			WHERE content_hash <> '' AND id IN (SELECT DISTINCT doc_id FROM knowledge_chunks)`,
		fmt.Sprintf(`UPDATE knowledge_collections SET dim = %d, updated_at = NOW()
			WHERE embedding_model <> '' AND dim <> %d`, dim, dim),
		fmt.Sprintf(`ALTER TABLE knowledge_chunks ALTER COLUMN embedding TYPE vector(%d)`, dim),
		knowledgeHNSWIndexSQL(),
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("knowledge dim reconcile %d→%d: %w", cur, dim, err)
		}
	}
	var affected int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge_documents WHERE status = 'pending'`).Scan(&affected)

	lg.Error("knowledge embedding dimension reconciled; stale embeddings voided, affected documents reset for re-embed",
		loggateway.StepID("data.schema.knowledge_dim_reconcile"),
		loggateway.Int("from_dim", cur), loggateway.Int("to_dim", dim), loggateway.Int("pending_docs", affected))
	return nil
}
