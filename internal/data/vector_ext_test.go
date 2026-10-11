package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type recordingExecer struct {
	queries []string
	err     error
}

func (f *recordingExecer) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	f.queries = append(f.queries, query)
	return nil, f.err
}

// TestEnsurePgVectorExtension is the fresh-database first-boot regression:
// NewData used to construct the vector store BEFORE any step created the
// pgvector extension (the P1 migration creates it later), so a brand-new
// database silently degraded vector search until restart. The startup helper
// must issue the idempotent DDL before NewPgVectorStore probes the vector
// type.
func TestEnsurePgVectorExtension(t *testing.T) {
	f := &recordingExecer{}
	if err := ensurePgVectorExtension(context.Background(), f); err != nil {
		t.Fatalf("ensurePgVectorExtension: %v", err)
	}
	if len(f.queries) != 1 {
		t.Fatalf("queries=%d, want exactly 1", len(f.queries))
	}
	if q := f.queries[0]; !strings.Contains(q, "CREATE EXTENSION IF NOT EXISTS vector") {
		t.Fatalf("unexpected DDL: %s", q)
	}
}

func TestEnsurePgVectorExtension_PropagatesError(t *testing.T) {
	f := &recordingExecer{err: errors.New("boom")}
	if err := ensurePgVectorExtension(context.Background(), f); err == nil {
		t.Fatal("want error propagated so the caller can warn and degrade")
	}
}
