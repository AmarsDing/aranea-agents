package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"

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
)

// EnsureKnowledgeSchema creates the knowledge tables and indexes when they do not exist.
func EnsureKnowledgeSchema(ctx context.Context, db *sql.DB, dim int) error {
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
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("knowledge schema: %w", err)
		}
	}
	return nil
}

// --- Collection CRUD ---

func (r *knowledgeRepo) CreateCollection(ctx context.Context, c biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	q := `INSERT INTO knowledge_collections
		(id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,0,$7,$8,$8)
		RETURNING id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		          to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`
	row := r.data.Postgres().QueryRowContext(ctx, q, c.ID, c.Name, c.Description, c.EmbeddingModel, c.Dim, c.Status, c.Workspace, now)
	return scanCollection(row)
}

func (r *knowledgeRepo) GetCollection(ctx context.Context, id string) (biz.KnowledgeCollection, error) {
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE id = $1`
	return scanCollection(r.data.Postgres().QueryRowContext(ctx, q, id))
}

func (r *knowledgeRepo) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]biz.KnowledgeCollection, int, error) {
	var total int
	cq := `SELECT COUNT(*) FROM knowledge_collections WHERE workspace = $1 OR $1 = ''`
	if err := r.data.Postgres().QueryRowContext(ctx, cq, workspace).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id, name, description, embedding_model, dim, status, document_count, chunk_count, workspace,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_collections WHERE workspace = $1 OR $1 = ''
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
	q := `INSERT INTO knowledge_documents
		(id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,0,$6,'',$7,$7)
		RETURNING id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		          to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`
	row := r.data.Postgres().QueryRowContext(ctx, q, d.ID, d.CollectionID, d.Source, d.MimeType, d.SizeBytes, d.Status, now)
	return scanDocument(row)
}

func (r *knowledgeRepo) GetDocument(ctx context.Context, id string) (biz.KnowledgeDocument, error) {
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
		         to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		         to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM knowledge_documents WHERE id = $1`
	return scanDocument(r.data.Postgres().QueryRowContext(ctx, q, id))
}

func (r *knowledgeRepo) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents
		 SET status = $2, error_message = $3, chunk_count = $4, updated_at = NOW()
		 WHERE id = $1`, id, status, errMsg, chunkCount)
	return err
}

func (r *knowledgeRepo) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]biz.KnowledgeDocument, int, error) {
	var total int
	if err := r.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_documents WHERE collection_id = $1 OR $1 = ''`, collectionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id, collection_id, source, mime_type, size_bytes, chunk_count, status, error_message,
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
		d, err := scanDocument(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
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
		switch err := tx.QueryRowContext(ctx,
			`SELECT collection_id, chunk_count, status FROM knowledge_documents WHERE id = $1`, id).
			Scan(&collectionID, &chunkCount, &status); {
		case err == sql.ErrNoRows:
			return nil
		case err != nil:
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
			vec := pgvector.NewVector(ch.Embedding)
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

func (r *knowledgeRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, queryEmbedding []float32) ([]biz.KnowledgeChunk, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("embedding is empty")
	}
	vec := pgvector.NewVector(queryEmbedding)
	scoreFilter := ""
	hasMinScore := q.MinScore > 0
	if hasMinScore {
		scoreFilter = "AND (1 - (embedding <=> $1::vector)) >= "
	}
	filterClause := ""
	filterArg := json.RawMessage("{}")
	if q.FilterJSON != "" {
		filterClause = "AND metadata @> $4::jsonb"
		filterArg = json.RawMessage(q.FilterJSON)
	}
	if hasMinScore {
		if filterClause != "" {
			scoreFilter += "$5"
		} else {
			scoreFilter += "$4"
		}
	}
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       (1 - (embedding <=> $1::vector)) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  %s
  %s
ORDER BY embedding <=> $1::vector
LIMIT $3`, scoreFilter, filterClause)

	var rows *sql.Rows
	var err error
	switch {
	case filterClause != "" && q.MinScore > 0:
		rows, err = r.data.Postgres().QueryContext(ctx, raw, vec, q.CollectionID, q.TopK, filterArg, q.MinScore)
	case filterClause != "":
		rows, err = r.data.Postgres().QueryContext(ctx, raw, vec, q.CollectionID, q.TopK, filterArg)
	case q.MinScore > 0:
		rows, err = r.data.Postgres().QueryContext(ctx, raw, vec, q.CollectionID, q.TopK, q.MinScore)
	default:
		rows, err = r.data.Postgres().QueryContext(ctx, raw, vec, q.CollectionID, q.TopK)
	}
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
	filterClause := ""
	filterArgIdx := 3
	filterArg := json.RawMessage("{}")
	if q.FilterJSON != "" {
		filterClause = fmt.Sprintf("AND metadata @> $%d::jsonb", filterArgIdx)
		filterArg = json.RawMessage(q.FilterJSON)
		filterArgIdx++
	}

	trgmResults, trgmErr := r.searchChunksTrigram(ctx, q, filterClause, filterArg, filterArgIdx)
	tsResults, tsErr := r.searchChunksTsvector(ctx, q, filterClause, filterArg, filterArgIdx)

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

func (r *knowledgeRepo) searchChunksTsvector(ctx context.Context, q biz.KnowledgeSearchQuery, filterClause string, filterArg json.RawMessage, nextArgIdx int) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', $1)) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  AND to_tsvector('simple', content) @@ plainto_tsquery('simple', $1)
  %s
ORDER BY score DESC
LIMIT $%d`, filterClause, nextArgIdx)

	var rows *sql.Rows
	var err error
	if filterClause != "" {
		rows, err = r.data.Postgres().QueryContext(ctx, raw, q.Query, q.CollectionID, filterArg, q.TopK)
	} else {
		rows, err = r.data.Postgres().QueryContext(ctx, raw, q.Query, q.CollectionID, q.TopK)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunks(rows)
}

func (r *knowledgeRepo) searchChunksTrigram(ctx context.Context, q biz.KnowledgeSearchQuery, filterClause string, filterArg json.RawMessage, nextArgIdx int) ([]biz.KnowledgeChunk, error) {
	raw := fmt.Sprintf(`
SELECT id, doc_id, collection_id, content, metadata::text, chunk_index,
       similarity(content, $1) AS score
FROM knowledge_chunks
WHERE collection_id = $2
  AND content %% $1
  %s
ORDER BY score DESC
LIMIT $%d`, filterClause, nextArgIdx)

	var rows *sql.Rows
	var err error
	if filterClause != "" {
		rows, err = r.data.Postgres().QueryContext(ctx, raw, q.Query, q.CollectionID, filterArg, q.TopK)
	} else {
		rows, err = r.data.Postgres().QueryContext(ctx, raw, q.Query, q.CollectionID, q.TopK)
	}
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
		&c.Status, &c.DocumentCount, &c.ChunkCount, &c.Workspace, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func scanDocument(row scannable) (biz.KnowledgeDocument, error) {
	var d biz.KnowledgeDocument
	err := row.Scan(&d.ID, &d.CollectionID, &d.Source, &d.MimeType, &d.SizeBytes,
		&d.ChunkCount, &d.Status, &d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
