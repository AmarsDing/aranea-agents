package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

type mockFactEmbedder struct{}

func (mockFactEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

type mockFactVectorRepo struct {
	calls int
}

func (m *mockFactVectorRepo) Insert(context.Context, *biz.AgentMemory) error { return nil }
func (m *mockFactVectorRepo) FindSimilar(context.Context, string, []float32, int) ([]*biz.AgentMemory, error) {
	return nil, nil
}
func (m *mockFactVectorRepo) FindSimilarWithUser(context.Context, string, string, []float32, int) ([]*biz.AgentMemory, error) {
	return nil, nil
}
func (m *mockFactVectorRepo) UpsertFactVector(context.Context, string, string, string, string, []float32) error {
	m.calls++
	return nil
}

func TestMemoryFactIndexSync_DualWrite(t *testing.T) {
	repo := &mockFactVectorRepo{}
	vec := biz.NewMemoryUsecase(repo, mockFactEmbedder{})
	db, d := openFactEmbedTestData(t)
	sync := NewMemoryFactIndexSync(vec, d, loggateway.NewNoop())
	if err := sync.SyncFactIndex(context.Background(), "agent-1", "u1", "fact-1", "User prefers dark mode"); err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 {
		t.Fatalf("expected pgvector upsert, got %d calls", repo.calls)
	}
	rows, err := db.QueryContext(context.Background(),
		`SELECT embedding_status, length(embedding_blob) FROM memory_facts WHERE id = 'fact-1'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("missing fact row")
	}
	var status string
	var blobLen int
	if err := rows.Scan(&status, &blobLen); err != nil {
		t.Fatal(err)
	}
	if status != "fresh" || blobLen != 16 {
		t.Fatalf("expected fresh blob len 16, got status=%q len=%d", status, blobLen)
	}
}

// mockFactBatchEmbedder implements biz.EmbeddingBatchService and counts calls
// so batch-sync tests can assert a single batched embed round-trip.
type mockFactBatchEmbedder struct {
	batchCalls int
	sizes      []int
}

func (m *mockFactBatchEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func (m *mockFactBatchEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	m.batchCalls++
	m.sizes = append(m.sizes, len(texts))
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3, 0.4}
	}
	return out, nil
}

// 2026-11-01: SyncFactIndexBatchFromRows syncs a whole sweep batch with one
// EmbedBatch round-trip (EmbeddingBatchService fast path), marking every row
// fresh; per-row writes still hit the pgvector upsert individually.
func TestMemoryFactIndexSync_BatchFromRows(t *testing.T) {
	repo := &mockFactVectorRepo{}
	emb := &mockFactBatchEmbedder{}
	vec := biz.NewMemoryUsecase(repo, emb)
	db, d := openFactEmbedTestData(t)
	ctx := context.Background()
	for _, id := range []string{"fact-b1", "fact-b2", "fact-b3"} {
		if _, err := db.ExecContext(ctx, `
INSERT INTO memory_facts (id, scope_type, scope_id, agent_id, user_id, statement, statement_normalized, fingerprint, status, embedding_status, created_at, updated_at)
VALUES ($1, 'agent', 'agent-1', 'agent-1', 'u1', $2, $3, $4, 'active', 'stale', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			id, "statement for "+id, "statement for "+id, "fp-"+id); err != nil {
			t.Fatal(err)
		}
	}
	sync := NewMemoryFactIndexSync(vec, d, loggateway.NewNoop())
	batcher, ok := sync.(biz.MemoryFactIndexBatchSyncer)
	if !ok {
		t.Fatal("memoryFactIndexSync must implement biz.MemoryFactIndexBatchSyncer")
	}
	raws := [][]byte{
		[]byte(`{"id":"fact-b1","agent_id":"agent-1","user_id":"u1","statement":"statement for fact-b1"}`),
		[]byte(`{"id":"fact-b2","agent_id":"agent-1","user_id":"u1","statement":"statement for fact-b2"}`),
		[]byte(`{"id":"fact-b3","agent_id":"agent-1","user_id":"u1","statement":"statement for fact-b3"}`),
	}
	synced, err := batcher.SyncFactIndexBatchFromRows(ctx, raws)
	if err != nil {
		t.Fatal(err)
	}
	if synced != 3 {
		t.Fatalf("expected 3 rows synced, got %d", synced)
	}
	if emb.batchCalls != 1 || emb.sizes[0] != 3 {
		t.Fatalf("expected one EmbedBatch call of 3 texts, got calls=%d sizes=%v", emb.batchCalls, emb.sizes)
	}
	if repo.calls != 3 {
		t.Fatalf("expected 3 pgvector upserts, got %d", repo.calls)
	}
	var freshCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM memory_facts WHERE id IN ('fact-b1','fact-b2','fact-b3') AND embedding_status = 'fresh' AND embedding_blob IS NOT NULL`).
		Scan(&freshCount); err != nil {
		t.Fatal(err)
	}
	if freshCount != 3 {
		t.Fatalf("expected 3 fresh rows with blobs, got %d", freshCount)
	}
}

// Batch embed failure marks every parsed row stale and bumps index_attempts,
// so the sweeper backoff paces the retry instead of hot-looping.
func TestMemoryFactIndexSync_BatchEmbedFailureMarksStale(t *testing.T) {
	repo := &mockFactVectorRepo{}
	vec := biz.NewMemoryUsecase(repo, &mockFactBatchFailEmbedder{})
	db, d := openFactEmbedTestData(t)
	ctx := context.Background()
	sync := NewMemoryFactIndexSync(vec, d, loggateway.NewNoop())
	batcher := sync.(biz.MemoryFactIndexBatchSyncer)
	raws := [][]byte{
		[]byte(`{"id":"fact-1","agent_id":"agent-1","user_id":"u1","statement":"User prefers dark mode"}`),
	}
	synced, err := batcher.SyncFactIndexBatchFromRows(ctx, raws)
	if err == nil {
		t.Fatal("expected embed failure error")
	}
	if synced != 0 {
		t.Fatalf("expected 0 synced, got %d", synced)
	}
	var status string
	var attempts int
	if err := db.QueryRowContext(ctx,
		`SELECT embedding_status, index_attempts FROM memory_facts WHERE id = 'fact-1'`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "stale" || attempts != 1 {
		t.Fatalf("expected stale/1 attempt, got status=%q attempts=%d", status, attempts)
	}
}

type mockFactBatchFailEmbedder struct{}

func (mockFactBatchFailEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, biz.ErrMemoryUnavailable
}

func (mockFactBatchFailEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, biz.ErrMemoryUnavailable
}

func openFactEmbedTestData(t *testing.T) (*sql.DB, *Data) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE memory_facts (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '',
 statement TEXT NOT NULL, statement_normalized TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', details_markdown TEXT NOT NULL DEFAULT '',
 fact_kind TEXT NOT NULL DEFAULT 'fact', tags_json TEXT NOT NULL DEFAULT '[]',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 use_count INTEGER NOT NULL DEFAULT 0, hit_count INTEGER NOT NULL DEFAULT 0,
 positive_feedback_count INTEGER NOT NULL DEFAULT 0, negative_feedback_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
 source_kind TEXT NOT NULL DEFAULT 'manual', source_episode_id TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', source_external TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active', superseded_by TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BYTEA, embedding_norm REAL NOT NULL DEFAULT 0, index_attempts INTEGER NOT NULL DEFAULT 0,
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', quality_score REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
valid_from TEXT NOT NULL DEFAULT '', valid_until TEXT NOT NULL DEFAULT '',
UNIQUE(scope_type, scope_id, fingerprint))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO memory_facts (id, scope_type, scope_id, agent_id, user_id, statement, statement_normalized, fingerprint, status, created_at, updated_at)
VALUES ('fact-1', 'agent', 'agent-1', 'agent-1', 'u1', 'User prefers dark mode', 'user prefers dark mode', 'fp1', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return db, d
}
