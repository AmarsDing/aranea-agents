package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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

var _ bizknowledge.IngestCommitter = (*knowledgeRepo)(nil)

var (
	_ biz.KnowledgeRepo           = (*knowledgeRepo)(nil)
	_ biz.KnowledgeSparseSearcher = (*knowledgeRepo)(nil)
	_ bizknowledge.Repo           = (*knowledgeRepo)(nil)
	// SP2 #9 embedding 熔断端口。
	_ bizknowledge.EmbedCircuitRepo = (*knowledgeRepo)(nil)
)

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

func (r *knowledgeRepo) GetCollectionByName(ctx context.Context, workspace, name string) (biz.KnowledgeCollection, error) {
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         root_path, sync_state, vault_backend, COALESCE(to_char(last_sync_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE workspace = $1 AND name = $2`
	return scanCollection(r.data.Postgres().QueryRowContext(ctx, q, workspace, name))
}

func (r *knowledgeRepo) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]biz.KnowledgeCollection, int, error) {
	var total int

	cq := `SELECT COUNT(*) FROM knowledge_collections WHERE workspace = $1 OR workspace = '' OR $1 = ''`
	if err := r.data.Postgres().QueryRowContext(ctx, cq, workspace).Scan(&total); err != nil {
		return nil, 0, entErrToBizErr(err, "knowledge")
	}
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         root_path, sync_state, vault_backend, COALESCE(to_char(last_sync_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE workspace = $1 OR workspace = '' OR $1 = ''
		  ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.data.Postgres().QueryContext(ctx, q, workspace, limit, offset)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "knowledge")
	}
	defer rows.Close()
	var out []biz.KnowledgeCollection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "knowledge")
		}
		out = append(out, c)
	}
	return out, total, entErrToBizErr(rows.Err(), "knowledge")
}

func (r *knowledgeRepo) DeleteCollection(ctx context.Context, id string) error {
	_, err := r.data.Postgres().ExecContext(ctx, `DELETE FROM knowledge_collections WHERE id = $1`, id)
	return entErrToBizErr(err, "knowledge")
}

func (r *knowledgeRepo) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_collections
		 SET document_count = document_count + $2,
		     chunk_count    = chunk_count    + $3,
		     updated_at     = NOW()
		 WHERE id = $1`, id, docDelta, chunkDelta)
	return entErrToBizErr(err, "knowledge")
}

// UpdateCollectionSyncState 回写 vault 同步状态与最近一次同步完成时间（P1-3 轮询）。
// lastSyncAt 为零值时只更新 state（失败场景不刷新完成时间）。
func (r *knowledgeRepo) UpdateCollectionSyncState(ctx context.Context, id, state string, lastSyncAt time.Time) error {
	if lastSyncAt.IsZero() {
		_, err := r.data.Postgres().ExecContext(ctx,
			`UPDATE knowledge_collections SET sync_state = $2, updated_at = NOW() WHERE id = $1`, id, state)
		return entErrToBizErr(err, "knowledge")
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_collections SET sync_state = $2, last_sync_at = $3, updated_at = NOW() WHERE id = $1`,
		id, state, lastSyncAt.UTC())
	return entErrToBizErr(err, "knowledge")
}

// EnableCollectionSemantic 空语义层单向启用（B2）：守卫式 UPDATE，仅当集合
// embedding_model 仍为空时绑定 model/dim；返回 bool=是否生效
// （RowsAffected=0 → false：并发已绑定或集合不存在，service 层已先做存在性检查）。
func (r *knowledgeRepo) EnableCollectionSemantic(ctx context.Context, id, model string, dim int) (bool, error) {
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE knowledge_collections SET embedding_model = $2, dim = $3, updated_at = NOW()
		 WHERE id = $1 AND embedding_model = ''`, id, model, dim)
	if err != nil {
		return false, entErrToBizErr(err, "knowledge")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "knowledge")
	}
	return affected > 0, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanCollection(row scannable) (biz.KnowledgeCollection, error) {
	var c biz.KnowledgeCollection
	err := row.Scan(&c.ID, &c.Name, &c.Description, &c.EmbeddingModel, &c.Dim,
		&c.Status, &c.DocumentCount, &c.ChunkCount, &c.Workspace,
		&c.RootPath, &c.SyncState, &c.VaultBackend, &c.LastSyncAt, &c.CreatedAt, &c.UpdatedAt)
	return c, entErrToBizErr(err, "knowledge")
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
