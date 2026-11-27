//go:build pgvector

package vector

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestPgVectorStore_SearchFilteredScopesByMeta pins the shared read index
// contract used by L2/L3 recall: a filtered search returns only the rows of
// that user (or agent), ordered by similarity, even when another user's row
// is the global nearest neighbour — the case that crowded single users out
// of a global top-K in a table shared by many users.
func TestPgVectorStore_SearchFilteredScopesByMeta(t *testing.T) {
	_, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	vs, err := NewPgVectorStore(db, "vector_embeddings", 3, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewPgVectorStore: %v", err)
	}
	s := vs.(*PgVectorStore)
	up := func(id, owner string, emb []float64) {
		t.Helper()
		if err := s.Upsert(ctx, id, emb, map[string]string{"agent_id": owner, "user_id": owner, "content": id}); err != nil {
			t.Fatalf("Upsert(%s): %v", id, err)
		}
	}
	up("b-best", "user-b", []float64{1, 0, 0})
	up("a-near", "user-a", []float64{0.9, 0.1, 0})
	up("a-far", "user-a", []float64{0.2, 1, 0})
	query := []float64{1, 0, 0}

	global, err := s.Search(ctx, query, 1, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(global) != 1 || global[0].ID != "b-best" {
		t.Fatalf("global top-1=%v, want b-best (another user's row wins unfiltered)", global)
	}

	hits, err := s.SearchFiltered(ctx, query, 5, 0, map[string]string{"user_id": "user-a"})
	if err != nil {
		t.Fatalf("SearchFiltered(user_id): %v", err)
	}
	if len(hits) != 2 || hits[0].ID != "a-near" || hits[1].ID != "a-far" {
		t.Fatalf("user-a hits=%v, want [a-near a-far]", hits)
	}
	if hits[0].Meta["user_id"] != "user-a" {
		t.Fatalf("hit meta=%v, want user_id=user-a", hits[0].Meta)
	}

	byAgent, err := s.SearchFiltered(ctx, query, 5, 0, map[string]string{"agent_id": "user-b"})
	if err != nil {
		t.Fatalf("SearchFiltered(agent_id): %v", err)
	}
	if len(byAgent) != 1 || byAgent[0].ID != "b-best" {
		t.Fatalf("agent user-b hits=%v, want [b-best]", byAgent)
	}

	floor, err := s.SearchFiltered(ctx, query, 5, 0.9, map[string]string{"user_id": "user-a"})
	if err != nil {
		t.Fatalf("SearchFiltered(minScore): %v", err)
	}
	if len(floor) != 1 || floor[0].ID != "a-near" {
		t.Fatalf("minScore 0.9 hits=%v, want [a-near]", floor)
	}

	if _, err := s.SearchFiltered(ctx, query, 5, 0, map[string]string{"user_id' OR '1'='1": "x"}); err == nil {
		t.Fatal("invalid meta key accepted; want an error")
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename = 'vector_embeddings'
		AND indexname IN ('vector_embeddings_meta_user_id_idx', 'vector_embeddings_meta_agent_id_idx')`).Scan(&n); err != nil {
		t.Fatalf("count filter indexes: %v", err)
	}
	if n != 2 {
		t.Fatalf("meta filter indexes=%d, want 2", n)
	}

	// Re-running ensureTable on an existing table is a no-op (startup path).
	if err := s.ensureTable(ctx); err != nil {
		t.Fatalf("ensureTable rerun: %v", err)
	}
}
