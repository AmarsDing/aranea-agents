package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

type knowledgeRepo struct {
	data *Data
	lg   loggateway.Logger
}

func NewKnowledgeRepo(data *Data, lg loggateway.Logger) biz.KnowledgeRepo {
	if data == nil || data.Postgres() == nil {
		return nil
	}
	return &knowledgeRepo{data: data, lg: lg}
}

func ivfflatLists(dim int) int {
	if v := os.Getenv("KRATOS_KNOWLEDGE_IVFFLAT_LISTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if dim <= 0 {
		return 100
	}
	lists := dim / 4
	if lists < 10 {
		lists = 10
	}
	if lists > 1000 {
		lists = 1000
	}
	return lists
}

var (
	_ biz.KnowledgeRepo           = (*knowledgeRepo)(nil)
	_ biz.KnowledgeSparseSearcher = (*knowledgeRepo)(nil)
	_ bizknowledge.Repo           = (*knowledgeRepo)(nil)
	// SP2 #9 embedding 熔断端口。
	_ bizknowledge.EmbedCircuitRepo = (*knowledgeRepo)(nil)
)

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
		// 统一摄取管线（Phase 8/9）：整理后全文 / LLM 整理标记 / 原始文件血缘。
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
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_idx
			ON knowledge_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = %d)`, ivfflatLists(dim)),
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_collection_idx
			ON knowledge_chunks(collection_id)`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_content_tsvector_idx
			ON knowledge_chunks USING GIN (to_tsvector('simple', content))`,
		`CREATE INDEX IF NOT EXISTS knowledge_chunks_content_trgm_idx
			ON knowledge_chunks USING GIN (content gin_trgm_ops)`,
		// --- Vault 升级（P1-1）：Collection → Vault，root_path 唯一（非空时） ---
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS root_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS sync_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_collections_root_path_key
			ON knowledge_collections (root_path) WHERE root_path <> ''`,
		// --- Vault 文档镜像列（P1）：rel_path/content_hash 支撑同步；summary/tags/doc_type 支撑摘要卡 ---
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS rel_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS summary_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS tags JSONB`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS doc_type TEXT NOT NULL DEFAULT ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS knowledge_documents_rel_path_key
			ON knowledge_documents (collection_id, rel_path) WHERE rel_path <> ''`,
		// --- 双轨关联（P2，派生索引纪律：可无状态重建，无业务表耦合） ---
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
		// --- SP1-F 团队库后端维度（fresh 形态；存量库由迁移 20261205 补列） ---
		// vault_backend：local=文件系统真相源（root_path 必填）/ team=PG 真相源（root_path 空，无 SyncEngine）。
		`ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS vault_backend TEXT NOT NULL DEFAULT 'local'`,
		// --- SP2 #9 embedding 熔断（fresh 形态；存量库由迁移 20261206 补列） ---
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_fail_count INT NOT NULL DEFAULT 0`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS embed_last_tried TIMESTAMPTZ`,
		`CREATE INDEX IF NOT EXISTS knowledge_documents_embed_degraded_idx
			ON knowledge_documents (collection_id, embed_last_tried) WHERE embed_fail_count > 0`,
		// --- SP1-C 跨库双链解析（fresh 形态；存量库由迁移 20261204 补列） ---
		// documents.title/aliases：Resolver 文档键（frontmatter 物化）；
		// links.weight：N-3 投影权重（同文档对块边数聚合）。
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS aliases JSONB`,
		`ALTER TABLE knowledge_links ADD COLUMN IF NOT EXISTS weight INT NOT NULL DEFAULT 1`,
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
//  4. ALTER 列类型 + 重建 ivfflat 索引（lists 随新维度重算；先 DROP 再 ALTER，
//     避免 PG 级联重建沿用旧 lists 参数）。
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
		`UPDATE knowledge_chunks SET embedding = NULL WHERE embedding IS NOT NULL`,
		`UPDATE knowledge_documents SET content_hash = '', status = 'pending', updated_at = NOW()
			WHERE content_hash <> '' AND id IN (SELECT DISTINCT doc_id FROM knowledge_chunks)`,
		fmt.Sprintf(`UPDATE knowledge_collections SET dim = %d, updated_at = NOW()
			WHERE embedding_model <> '' AND dim <> %d`, dim, dim),
		fmt.Sprintf(`ALTER TABLE knowledge_chunks ALTER COLUMN embedding TYPE vector(%d)`, dim),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_idx
			ON knowledge_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = %d)`, ivfflatLists(dim)),
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("knowledge dim reconcile %d→%d: %w", cur, dim, err)
		}
	}
	var affected int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge_documents WHERE status = 'pending'`).Scan(&affected)
	// Error 级：对账作废了存量向量，运维必须感知（K3 降级/数据愈合节点）。
	lg.Error("knowledge embedding dimension reconciled; stale embeddings voided, affected documents reset for re-embed",
		loggateway.StepID("data.schema.knowledge_dim_reconcile"),
		loggateway.Int("from_dim", cur), loggateway.Int("to_dim", dim), loggateway.Int("pending_docs", affected))
	return nil
}

// --- Collection CRUD ---

func (r *knowledgeRepo) CreateCollection(ctx context.Context, c biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	q := `INSERT INTO knowledge_collections
		(id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		 root_path, sync_state, vault_backend, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,0,$7,$8,$9,$10,$11,$11)
		RETURNING id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		          root_path, sync_state, vault_backend, COALESCE(to_char(last_sync_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		          to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`
	row := r.data.Postgres().QueryRowContext(ctx, q,
		c.ID, c.Name, c.Description, c.EmbeddingModel, c.Dim, c.Status, c.Workspace, c.RootPath, c.SyncState, c.VaultBackend, now)
	return scanCollection(row)
}

func (r *knowledgeRepo) GetCollection(ctx context.Context, id string) (biz.KnowledgeCollection, error) {
	// C-25: tenant sees own + shared (empty workspace); system sees all.
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         root_path, sync_state, vault_backend, COALESCE(to_char(last_sync_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE id = $1`
	args := []any{id}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (workspace = $2 OR workspace = '')`
		args = append(args, ws)
	}
	return scanCollection(r.data.Postgres().QueryRowContext(ctx, q, args...))
}

func (r *knowledgeRepo) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]biz.KnowledgeCollection, int, error) {
	var total int
	// C-01: when workspace is non-empty, return tenant-owned + shared (empty
	// workspace) rows. Empty workspace query (system) returns everything.
	cq := `SELECT COUNT(*) FROM knowledge_collections WHERE workspace = $1 OR workspace = '' OR $1 = ''`
	if err := r.data.Postgres().QueryRowContext(ctx, cq, workspace).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         root_path, sync_state, vault_backend, COALESCE(to_char(last_sync_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE workspace = $1 OR workspace = '' OR $1 = ''
		  ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.data.Postgres().QueryContext(ctx, q, workspace, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.KnowledgeCollection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *knowledgeRepo) DeleteCollection(ctx context.Context, id string) error {
	_, err := r.data.Postgres().ExecContext(ctx, `DELETE FROM knowledge_collections WHERE id = $1`, id)
	return err
}

func (r *knowledgeRepo) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_collections
		 SET document_count = document_count + $2,
		     chunk_count    = chunk_count    + $3,
		     updated_at     = NOW()
		 WHERE id = $1`, id, docDelta, chunkDelta)
	return err
}

// --- Document CRUD ---

func (r *knowledgeRepo) CreateDocument(ctx context.Context, d biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, err := marshalTags(d.Tags)
	if err != nil {
		return biz.KnowledgeDocument{}, err
	}
	q := `INSERT INTO knowledge_documents
		(id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message, content_text, organized, asset_uri,
		 rel_path, content_hash, summary, summary_hash, tags, doc_type, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,0,$6,'',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
		RETURNING id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
	          content_text, organized, asset_uri,
	          rel_path, content_hash, summary, summary_hash, tags, doc_type,
	          embed_fail_count, embed_last_tried,
	          to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`
	row := r.data.Postgres().QueryRowContext(ctx, q, d.ID, d.CollectionID, d.Source, d.MimeType, d.SizeBytes, d.Status,
		d.ContentText, d.Organized, d.AssetURI,
		d.RelPath, d.ContentHash, d.Summary, d.SummaryHash, tagsJSON, d.DocType, now)
	return scanDocument(row)
}

// GetDocumentByRelPath 按 vault 相对路径寻址文档（Vault 同步用，rel_path 唯一）。
func (r *knowledgeRepo) GetDocumentByRelPath(ctx context.Context, collectionID, relPath string) (biz.KnowledgeDocument, error) {
	q := `SELECT d.id, d.collection_id, d.source, d.mime_type, d.size_bytes, d.chunk_count, d.status, d.error_message,
		         d.content_text, d.organized, d.asset_uri,
		         d.rel_path, d.content_hash, d.summary, d.summary_hash, d.tags, d.doc_type,
		         d.embed_fail_count, d.embed_last_tried,
		         to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.collection_id = $1 AND d.rel_path = $2`
	args := []any{collectionID, relPath}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $3 OR c.workspace = '')`
		args = append(args, ws)
	}
	doc, err := scanDocument(r.data.Postgres().QueryRowContext(ctx, q, args...))
	if err != nil {
		// DB-R5：翻译 sql.ErrNoRows → CodeNotFound；vault sync applier 依赖
		// NotFound 判定走「创建新文档」路径，裸 ErrNoRows 会导致同步失败。
		return doc, entErrToBizErr(err, "KNOWLEDGE")
	}
	return doc, nil
}

// ListDocumentNames 批量解析文档显示名（SP1-E DocNameReader）：rel_path 优先，
// 空 rel_path 回 source；未知 id 缺席。空入参短路空 map（避免空数组查询）。
func (r *knowledgeRepo) ListDocumentNames(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, COALESCE(NULLIF(rel_path, ''), source) FROM knowledge_documents WHERE id = ANY($1)`,
		pq.Array(ids))
	if err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, entErrToBizErr(err, "knowledge")
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "knowledge")
	}
	return out, nil
}

// UpdateDocumentRelPath 文件移动/重命名时更新镜像路径（保留文档身份与索引）。
func (r *knowledgeRepo) UpdateDocumentRelPath(ctx context.Context, id, newRelPath string) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET rel_path = $2, updated_at = NOW() WHERE id = $1`, id, newRelPath)
	return err
}

// UpdateDocumentSyncMeta 文件内容变更时回写同步元数据（Vault 同步 modified 事件，P1-3）。
func (r *knowledgeRepo) UpdateDocumentSyncMeta(ctx context.Context, id string, meta biz.KnowledgeDocumentSyncMeta) error {
	tagsJSON, err := marshalTags(meta.Tags)
	if err != nil {
		return err
	}
	_, err = r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET content_hash = $2, summary = $3, summary_hash = $4, tags = $5, doc_type = $6, updated_at = NOW()
		 WHERE id = $1`, id, meta.ContentHash, meta.Summary, meta.SummaryHash, tagsJSON, meta.DocType)
	return err
}

// UpdateCollectionSyncState 回写 vault 同步状态与最近一次同步完成时间（P1-3 轮询）。
// lastSyncAt 为零值时只更新 state（失败场景不刷新完成时间）。
func (r *knowledgeRepo) UpdateCollectionSyncState(ctx context.Context, id, state string, lastSyncAt time.Time) error {
	if lastSyncAt.IsZero() {
		_, err := r.data.Postgres().ExecContext(ctx,
			`UPDATE knowledge_collections SET sync_state = $2, updated_at = NOW() WHERE id = $1`, id, state)
		return err
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_collections SET sync_state = $2, last_sync_at = $3, updated_at = NOW() WHERE id = $1`,
		id, state, lastSyncAt.UTC())
	return err
}

func (r *knowledgeRepo) GetDocument(ctx context.Context, id string) (biz.KnowledgeDocument, error) {
	// C-25: documents inherit collection workspace; filter via JOIN.
	q := `SELECT d.id, d.collection_id, d.source, d.mime_type, d.size_bytes, d.chunk_count, d.status, d.error_message,
		         d.content_text, d.organized, d.asset_uri,
		         d.rel_path, d.content_hash, d.summary, d.summary_hash, d.tags, d.doc_type,
		         d.embed_fail_count, d.embed_last_tried,
		         to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents d
		  JOIN knowledge_collections c ON c.id = d.collection_id
		  WHERE d.id = $1`
	args := []any{id}
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		q += ` AND (c.workspace = $2 OR c.workspace = '')`
		args = append(args, ws)
	}
	doc, err := scanDocument(r.data.Postgres().QueryRowContext(ctx, q, args...))
	if err != nil {
		return doc, entErrToBizErr(err, "KNOWLEDGE")
	}
	return doc, nil
}

func (r *knowledgeRepo) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET status = $2, error_message = $3, chunk_count = $4, updated_at = NOW()
		 WHERE id = $1`, id, status, errMsg, chunkCount)
	return err
}

// UpdateDocumentContent 回写文档正文与整理标记（Phase 9 图片异步提取完成后调用）。
func (r *knowledgeRepo) UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET content_text = $2, organized = $3, updated_at = NOW()
		 WHERE id = $1`, id, contentText, organized)
	return err
}

// --- SP2 #9 embedding 熔断端口（bizknowledge.EmbedCircuitRepo） ---

// UpdateDocumentEmbedCircuit 回写文档熔断状态。failCount=0 表示复位（lastTried 清 NULL）。
func (r *knowledgeRepo) UpdateDocumentEmbedCircuit(ctx context.Context, id string, failCount int, lastTried time.Time) error {
	var tried any
	if !lastTried.IsZero() {
		tried = lastTried.UTC()
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET embed_fail_count = $2, embed_last_tried = $3, updated_at = NOW()
		 WHERE id = $1`, id, failCount, tried)
	return entErrToBizErr(err, "KNOWLEDGE")
}

// ListEmbedDegradedDocuments 列出熔断中（failCount>0）的文档，按退避起点升序
// （最久未尝试者优先）。含 content_text 供重分块回填向量。
func (r *knowledgeRepo) ListEmbedDegradedDocuments(ctx context.Context, collectionID string, limit int) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         content_text, organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         embed_fail_count, embed_last_tried,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents
		  WHERE collection_id = $1 AND embed_fail_count > 0
		  ORDER BY embed_last_tried ASC NULLS FIRST
		  LIMIT $2`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, collectionID, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE")
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "KNOWLEDGE")
		}
		out = append(out, d)
	}
	return out, entErrToBizErr(rows.Err(), "KNOWLEDGE")
}

// UpdateChunkEmbeddings 恢复成功时按 chunk ID 精确寻址回填向量（ChunkIndex 源自
// splitter 元数据，不保证连续，禁止按位置映射）。docID 作为归属守卫防止跨文档误写。
func (r *knowledgeRepo) UpdateChunkEmbeddings(ctx context.Context, docID string, vecs []bizknowledge.ChunkEmbedding) error {
	if len(vecs) == 0 {
		return nil
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`UPDATE knowledge_chunks SET embedding = $1 WHERE id = $2 AND doc_id = $3`)
		if err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		defer stmt.Close()
		for _, v := range vecs {
			var vec any
			if len(v.Embedding) > 0 {
				vec = pgvector.NewVector(v.Embedding)
			}
			if _, err := stmt.ExecContext(ctx, vec, v.ChunkID, docID); err != nil {
				return entErrToBizErr(err, "KNOWLEDGE")
			}
		}
		return nil
	})
}

func (r *knowledgeRepo) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]biz.KnowledgeDocument, int, error) {
	var total int
	if err := r.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_documents WHERE collection_id = $1 OR $1 = ''`, collectionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents WHERE collection_id = $1 OR $1 = ''
		  ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.data.Postgres().QueryContext(ctx, q, collectionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocumentSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// ListDocumentsPendingReembed 列出待重嵌入文档（B1）：有正文、非 indexing、
// 且（chunks embedding IS NULL 或无任何 chunks）。按 created_at ASC（先入队先处理）。
// 返回完整文档（含 content_text，scanDocument 全字段扫描）——重嵌入以 content_text
// 为正文源，无需原始文件（与 ListEmbedDegradedDocuments 同理带正文）。
func (r *knowledgeRepo) ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         content_text, organized, asset_uri,
		         rel_path, content_hash, summary, summary_hash, tags, doc_type,
		         embed_fail_count, embed_last_tried,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents d
		  WHERE collection_id = $1
		    AND content_text <> ''
		    AND status <> 'indexing'
		    AND (id IN (SELECT doc_id FROM knowledge_chunks WHERE embedding IS NULL)
		         OR NOT EXISTS (SELECT 1 FROM knowledge_chunks c WHERE c.doc_id = d.id))
		  ORDER BY created_at ASC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.KnowledgeDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDocument removes a document, lets the FK cascade drop its chunks, and
// atomically adjusts the owning collection's cached counters (DAT-02 / REV-B).
//
// Counter adjustment rules:
//   - document_count is only decremented when the document was successfully
//     indexed (status = "indexed"). For pending/indexing/error documents the
//     collection counter was never incremented, so decrementing would produce
//     drift.
//   - chunk_count is decremented by the document's chunk_count regardless of
//     status, but GREATEST guards against going below 0. (A partially-indexed
//     document that failed mid-way may have some chunks already counted.)
//
// All statements run in one transaction to avoid any partial-update window.
func (r *knowledgeRepo) DeleteDocument(ctx context.Context, id string) error {
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var collectionID string
		var chunkCount int
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT collection_id, chunk_count, status FROM knowledge_documents WHERE id = $1`, id).
			Scan(&collectionID, &chunkCount, &status)
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id = $1`, id); err != nil {
			return err
		}
		if collectionID != "" {
			docDelta := 0
			if status == "indexed" {
				docDelta = 1
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE knowledge_collections
				 SET document_count = GREATEST(document_count - $2, 0),
				     chunk_count    = GREATEST(chunk_count    - $3, 0),
				     updated_at     = NOW()
				 WHERE id = $1`, collectionID, docDelta, chunkCount); err != nil {
				r.lg.Warn("delete document counter update failed", loggateway.StepID("knowledge.counter_drift"), loggateway.Err(err))
				return err
			}
		}
		return nil
	})
}

func (r *knowledgeRepo) InsertChunks(ctx context.Context, chunks []biz.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	var expectedDim int
	err := r.data.Postgres().QueryRowContext(ctx,
		"SELECT dim FROM knowledge_collections WHERE id = $1", chunks[0].CollectionID).Scan(&expectedDim)
	if err != nil {
		return fmt.Errorf("failed to query collection dimension: %w", err)
	}
	for _, ch := range chunks {
		if expectedDim > 0 && len(ch.Embedding) != expectedDim {
			return fmt.Errorf("embedding dimension mismatch: collection expects %d, chunk %q has %d", expectedDim, ch.ID, len(ch.Embedding))
		}
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, embedding, metadata, chunk_index)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, ch := range chunks {
			meta := ch.MetadataJSON
			if meta == "" {
				meta = "{}"
			}
			// 无语义层 vault（R-4）：空 embedding 写 NULL，pgvector.NewVector(nil) 会生成非法 '[]'。
			var vec any
			if len(ch.Embedding) > 0 {
				vec = pgvector.NewVector(ch.Embedding)
			}
			if _, err := stmt.ExecContext(ctx, ch.ID, ch.DocID, ch.CollectionID, ch.Content, vec, meta, ch.ChunkIndex); err != nil {
				r.lg.Warn("chunk insert failed", loggateway.StepID("knowledge.chunk_insert_fail"), loggateway.Err(err))
				return err
			}
		}
		return nil
	})
}

func (r *knowledgeRepo) DeleteChunksByDocument(ctx context.Context, docID string) error {
	_, err := r.data.Postgres().ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE doc_id = $1`, docID)
	return err
}

// MoveDocument moves a document (and its chunks) to another collection in one
// transaction, keeping both collections' cached counters in sync (US-14).
//
// Counter rules mirror DeleteDocument: document_count shifts only when the
// document was successfully indexed (pending/indexing/error docs were never
// counted); chunk_count shifts by the document's recorded chunk_count,
// GREATEST-guarded on the source side against drift below zero.
func (r *knowledgeRepo) MoveDocument(ctx context.Context, id, targetCollectionID string) (biz.KnowledgeDocument, error) {
	err := r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var sourceCollectionID string
		var chunkCount int
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT collection_id, chunk_count, status FROM knowledge_documents WHERE id = $1`, id).
			Scan(&sourceCollectionID, &chunkCount, &status)
		if err != nil {
			return err
		}
		if sourceCollectionID == targetCollectionID {
			return nil // 同库 no-op（biz 已守卫，此处防御并发改写）
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_documents SET collection_id = $2, updated_at = NOW() WHERE id = $1`,
			id, targetCollectionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_chunks SET collection_id = $2 WHERE doc_id = $1`,
			id, targetCollectionID); err != nil {
			return err
		}
		docDelta := 0
		if status == "indexed" {
			docDelta = 1
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_collections
			 SET document_count = GREATEST(document_count - $2, 0),
			     chunk_count    = GREATEST(chunk_count    - $3, 0),
			     updated_at     = NOW()
			 WHERE id = $1`, sourceCollectionID, docDelta, chunkCount); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_collections
			 SET document_count = document_count + $2,
			     chunk_count    = chunk_count    + $3,
			     updated_at     = NOW()
			 WHERE id = $1`, targetCollectionID, docDelta, chunkCount); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return biz.KnowledgeDocument{}, err
	}
	return r.GetDocument(ctx, id)
}

// pathPrefixClause 生成 G3-B7 搜索范围过滤子查询：仅命中文档 rel_path 位于
// "<prefix>/" 下的 chunks（目录边界语义，防 "note" 误中 "notes/"）。首尾斜杠
// 归一；LIKE 通配符（% _ \）按字面值转义。返回空串 = 不过滤。
// collectionPH 是外层查询中 collection_id 的占位符（如 "$2"），argIdx 是本
// 子查询 LIKE 参数的占位序号。
func pathPrefixClause(prefix, collectionPH string, argIdx int) (clause string, arg any) {
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	if p == "" {
		return "", nil
	}
	var b strings.Builder
	b.Grow(len(p) + 2)
	for _, r := range p {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteString("/%")
	return fmt.Sprintf(`AND doc_id IN (SELECT id FROM knowledge_documents WHERE collection_id = %s AND rel_path LIKE $%d ESCAPE '\')`, collectionPH, argIdx), b.String()
}

func (r *knowledgeRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, queryEmbedding []float32) ([]biz.KnowledgeChunk, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("embedding is empty")
	}
	vec := pgvector.NewVector(queryEmbedding)
	args := []any{vec, q.CollectionID, q.TopK}
	clauses := ""
	if c, a := pathPrefixClause(q.PathPrefix, "$2", len(args)+1); c != "" {
		clauses += "\n  " + c
		args = append(args, a)
	}
	if q.FilterJSON != "" {
		clauses += fmt.Sprintf("\n  AND metadata @> $%d::jsonb", len(args)+1)
		args = append(args, json.RawMessage(q.FilterJSON))
	}
	if q.MinScore > 0 {
		clauses += fmt.Sprintf("\n  AND (1 - (embedding <=> $1::vector)) >= $%d", len(args)+1)
		args = append(args, q.MinScore)
	}
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       (1 - (embedding <=> $1::vector)) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  AND embedding IS NOT NULL
  %s
ORDER BY embedding <=> $1::vector
LIMIT $3`, clauses)

	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []biz.KnowledgeChunk
	for rows.Next() {
		var ch biz.KnowledgeChunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.Score); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (r *knowledgeRepo) SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	extraClauses := ""
	var extraArgs []any
	if c, a := pathPrefixClause(q.PathPrefix, "$2", 3+len(extraArgs)); c != "" {
		extraClauses += "\n  " + c
		extraArgs = append(extraArgs, a)
	}
	if q.FilterJSON != "" {
		extraClauses += fmt.Sprintf("\n  AND metadata @> $%d::jsonb", 3+len(extraArgs))
		extraArgs = append(extraArgs, json.RawMessage(q.FilterJSON))
	}

	trgmResults, trgmErr := r.searchChunksTrigram(ctx, q, extraClauses, extraArgs)
	tsResults, tsErr := r.searchChunksTsvector(ctx, q, extraClauses, extraArgs)

	if trgmErr != nil && tsErr != nil {
		return nil, trgmErr
	}
	if trgmErr != nil {
		return tsResults, nil
	}
	if tsErr != nil {
		return trgmResults, nil
	}

	return mergeBM25Results(tsResults, trgmResults, q.TopK), nil
}

func (r *knowledgeRepo) searchChunksTsvector(ctx context.Context, q biz.KnowledgeSearchQuery, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', $1)) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  AND to_tsvector('simple', content) @@ plainto_tsquery('simple', $1)
  %s
ORDER BY score DESC
LIMIT $%d`, extraClauses, 3+len(extraArgs))

	args := []any{q.Query, q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunks(rows)
}

func (r *knowledgeRepo) searchChunksTrigram(ctx context.Context, q biz.KnowledgeSearchQuery, extraClauses string, extraArgs []any) ([]biz.KnowledgeChunk, error) {
	// word_similarity($1, content)：查询 trigram 集 vs 文本连续区间的最大相似度
	// （2026-08-10 根修）。旧实现 similarity(content, $1) + `%` 对中文短查询失效——
	// 分母含长文本全部 trigram，2-4 字查询相似度被稀释到 0.3 阈值以下永不命中。
	// `%>` 是 `<%` 的交换操作符（doc %> query ≡ query <% doc），gin_trgm_ops 索引支持。
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       word_similarity($1, content) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  AND content %%> $1
  %s
ORDER BY score DESC
LIMIT $%d`, extraClauses, 3+len(extraArgs))

	args := []any{q.Query, q.CollectionID}
	args = append(args, extraArgs...)
	args = append(args, q.TopK)
	rows, err := r.data.Postgres().QueryContext(ctx, raw, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunks(rows)
}

func mergeBM25Results(tsResults, trgmResults []biz.KnowledgeChunk, topK int) []biz.KnowledgeChunk {
	seen := make(map[string]struct{}, len(tsResults)+len(trgmResults))
	merged := make([]biz.KnowledgeChunk, 0, len(tsResults)+len(trgmResults))
	for _, ch := range tsResults {
		if _, ok := seen[ch.ID]; !ok {
			seen[ch.ID] = struct{}{}
			merged = append(merged, ch)
		}
	}
	for _, ch := range trgmResults {
		if _, ok := seen[ch.ID]; ok {
			continue
		}
		seen[ch.ID] = struct{}{}
		merged = append(merged, ch)
	}
	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}

func scanChunks(rows *sql.Rows) ([]biz.KnowledgeChunk, error) {
	var out []biz.KnowledgeChunk
	for rows.Next() {
		var ch biz.KnowledgeChunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.CollectionID, &ch.Content, &ch.MetadataJSON, &ch.ChunkIndex, &ch.Score); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// --- scanner helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanCollection(row scannable) (biz.KnowledgeCollection, error) {
	var c biz.KnowledgeCollection
	err := row.Scan(&c.ID, &c.Name, &c.Description, &c.EmbeddingModel, &c.Dim,
		&c.Status, &c.DocumentCount, &c.ChunkCount, &c.Workspace,
		&c.RootPath, &c.SyncState, &c.VaultBackend, &c.LastSyncAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func scanDocument(row scannable) (biz.KnowledgeDocument, error) {
	var d biz.KnowledgeDocument
	var tagsRaw []byte
	var embedLastTried sql.NullTime
	err := row.Scan(&d.ID, &d.CollectionID, &d.Source, &d.MimeType, &d.SizeBytes,
		&d.ChunkCount, &d.Status, &d.ErrorMessage, &d.ContentText, &d.Organized, &d.AssetURI,
		&d.RelPath, &d.ContentHash, &d.Summary, &d.SummaryHash, &tagsRaw, &d.DocType,
		&d.EmbedFailCount, &embedLastTried,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	d.Tags = unmarshalTags(tagsRaw)
	if embedLastTried.Valid {
		d.EmbedLastTried = embedLastTried.Time
	}
	return d, nil
}

// scanDocumentSummary 用于列表查询：不取 content_text 大字段（避免列表带宽放大）。
func scanDocumentSummary(row scannable) (biz.KnowledgeDocument, error) {
	var d biz.KnowledgeDocument
	var tagsRaw []byte
	err := row.Scan(&d.ID, &d.CollectionID, &d.Source, &d.MimeType, &d.SizeBytes,
		&d.ChunkCount, &d.Status, &d.ErrorMessage, &d.Organized, &d.AssetURI,
		&d.RelPath, &d.ContentHash, &d.Summary, &d.SummaryHash, &tagsRaw, &d.DocType,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	d.Tags = unmarshalTags(tagsRaw)
	return d, nil
}

// marshalTags 序列化标签为 JSONB 参数（空为 NULL）。
func marshalTags(tags []string) (any, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return nil, apierror.Internal("knowledge", "marshal tags").WithCause(err)
	}
	return b, nil
}

func unmarshalTags(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	return tags
}
