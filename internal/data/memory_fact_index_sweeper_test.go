package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// recordingFactIndexSyncer captures the ids handed to SyncFactIndexFromRow so
// pickup-filter tests can assert exactly which rows the sweeper selected.
type recordingFactIndexSyncer struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingFactIndexSyncer) SyncFactIndex(context.Context, string, string, string, string) error {
	return nil
}

func (r *recordingFactIndexSyncer) SyncFactIndexFromRow(_ context.Context, raw []byte) error {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return biz.ErrMemoryUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, _ := m["id"].(string)
	r.ids = append(r.ids, id)
	return nil
}

func (r *recordingFactIndexSyncer) picked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func openSweeperTestData(t *testing.T) (*sql.DB, *Data) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	// Full column set of sqlFactSelect (the sweeper scans via scanFactRowJSON,
	// unlike memory_fact_index_sync_test.go which queries columns directly).
	if _, err := db.ExecContext(ctx, `CREATE TABLE memory_facts (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '',
 statement TEXT NOT NULL, statement_normalized TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', details_markdown TEXT NOT NULL DEFAULT '',
 fact_kind TEXT NOT NULL DEFAULT 'fact', tags_json TEXT NOT NULL DEFAULT '[]',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 use_count INTEGER NOT NULL DEFAULT 0, hit_count INTEGER NOT NULL DEFAULT 0,
 recalled_count INTEGER NOT NULL DEFAULT 0, injected_count INTEGER NOT NULL DEFAULT 0, cited_count INTEGER NOT NULL DEFAULT 0,
 positive_feedback_count INTEGER NOT NULL DEFAULT 0, negative_feedback_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
 source_kind TEXT NOT NULL DEFAULT 'manual', source_episode_id TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', source_external TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active', superseded_by TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BYTEA, embedding_norm REAL NOT NULL DEFAULT 0, index_attempts INTEGER NOT NULL DEFAULT 0,
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '', pii_types TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', quality_score REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 valid_from TEXT NOT NULL DEFAULT '', valid_until TEXT NOT NULL DEFAULT '',
 links TEXT NOT NULL DEFAULT '[]', keywords TEXT NOT NULL DEFAULT '[]', tags TEXT NOT NULL DEFAULT '[]',
 decay_score REAL NOT NULL DEFAULT 1.0, context_note TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, fingerprint))`); err != nil {
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

func insertSweeperFact(t *testing.T, db *sql.DB, id, embStatus string, attempts int, updatedAt, rowStatus string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO memory_facts (id, scope_type, scope_id, agent_id, user_id, statement, statement_normalized, fingerprint, status, embedding_status, index_attempts, created_at, updated_at)
 VALUES ($1, 'user', 'u1', 'agent-1', 'u1', $2, $3, $4, $5, $6, $7, '2020-01-01T00:00:00Z', $8)`,
		id, "statement for "+id, strings.ToLower("statement for "+id), "fp-"+id, rowStatus, embStatus, attempts, updatedAt)
	if err != nil {
		t.Fatal(err)
	}
}

// C3-①: a stale, due row is replayed through the real sync path and converges
// to the post-sync marker 'fresh' (syncEmbeddingBlob + markFactIndexSynced).
func TestMemoryFactIndexSweeper_StaleRowSynced(t *testing.T) {
	db, d := openSweeperTestData(t)
	insertSweeperFact(t, db, "sweep-1", "stale", 2, "2020-01-01T00:00:00Z", "active")

	repo := &mockFactVectorRepo{}
	vec := biz.NewMemoryUsecase(repo, mockFactEmbedder{})
	syncer := NewMemoryFactIndexSync(vec, d, loggateway.NewNoop())
	sw := NewMemoryFactIndexSweeper(d, syncer, loggateway.NewNoop(), time.Minute, 10, 8)
	if got := sw.sweepOnce(context.Background()); got != 1 {
		t.Fatalf("expected 1 row synced, got %d", got)
	}
	if repo.calls != 1 {
		t.Fatalf("expected pgvector upsert, got %d calls", repo.calls)
	}
	var status string
	var attempts, blobLen int
	if err := db.QueryRowContext(context.Background(),
		`SELECT embedding_status, index_attempts, COALESCE(length(embedding_blob), 0) FROM memory_facts WHERE id = 'sweep-1'`).
		Scan(&status, &attempts, &blobLen); err != nil {
		t.Fatal(err)
	}
	if status != "fresh" || attempts != 0 || blobLen != 16 {
		t.Fatalf("expected fresh/0-attempts/blob16, got status=%q attempts=%d len=%d", status, attempts, blobLen)
	}
}

// C3-②: rows at the attempt cap, still inside the backoff window, or not
// active are never picked; due stale/pending rows are.
func TestMemoryFactIndexSweeper_PickupFilters(t *testing.T) {
	db, d := openSweeperTestData(t)
	insertSweeperFact(t, db, "sweep-due", "stale", 1, "2020-01-01T00:00:00Z", "active")
	insertSweeperFact(t, db, "sweep-pending", "pending", 0, "2020-01-01T00:00:00Z", "active")
	insertSweeperFact(t, db, "sweep-max", "stale", 8, "2020-01-01T00:00:00Z", "active")                        // attempts == maxAttempts
	insertSweeperFact(t, db, "sweep-backoff", "stale", 1, time.Now().UTC().Format(time.RFC3339Nano), "active") // 2-min backoff not elapsed
	insertSweeperFact(t, db, "sweep-archived", "stale", 0, "2020-01-01T00:00:00Z", "archived")

	rec := &recordingFactIndexSyncer{}
	sw := NewMemoryFactIndexSweeper(d, rec, loggateway.NewNoop(), time.Minute, 10, 8)
	if got := sw.sweepOnce(context.Background()); got != 2 {
		t.Fatalf("expected 2 rows synced, got %d", got)
	}
	picked := map[string]bool{}
	for _, id := range rec.picked() {
		picked[id] = true
	}
	if !picked["sweep-due"] || !picked["sweep-pending"] {
		t.Fatalf("due rows not picked: %v", rec.picked())
	}
	for _, id := range []string{"sweep-max", "sweep-backoff", "sweep-archived"} {
		if picked[id] {
			t.Fatalf("row %s must not be picked, got %v", id, rec.picked())
		}
	}
	// The attempt-capped row stays exactly as inserted (no re-mark, no pickup).
	var status string
	var attempts int
	if err := db.QueryRowContext(context.Background(),
		`SELECT embedding_status, index_attempts FROM memory_facts WHERE id = 'sweep-max'`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "stale" || attempts != 8 {
		t.Fatalf("sweep-max row mutated: status=%q attempts=%d", status, attempts)
	}
}

// C3-③: Run honors ctx cancellation promptly and returns nil.
func TestMemoryFactIndexSweeper_RunStopsOnCancel(t *testing.T) {
	_, d := openSweeperTestData(t)
	rec := &recordingFactIndexSyncer{}
	sw := NewMemoryFactIndexSweeper(d, rec, loggateway.NewNoop(), 10*time.Millisecond, 10, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sw.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned non-nil error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after ctx cancel")
	}
}
