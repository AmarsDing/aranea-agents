package vector

import (
	"context"
	"database/sql"
	"math"
	"testing"

	_ "github.com/glebarez/go-sqlite/compat"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return db
}

func TestSQLiteVectorStore_UpsertSearchDelete(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store, err := NewSQLiteVectorStore(db, "test_vectors", nil)
	if err != nil {
		t.Fatalf("NewSQLiteVectorStore: %v", err)
	}
	ctx := context.Background()

	vec1 := []float64{1.0, 0.0, 0.0}
	vec2 := []float64{0.0, 1.0, 0.0}
	vec3 := []float64{0.9, 0.1, 0.0}

	if err := store.Upsert(ctx, "v1", vec1, map[string]string{"kind": "fact"}); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	if err := store.Upsert(ctx, "v2", vec2, map[string]string{"kind": "episode"}); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}

	// Search for vectors similar to vec3 (should be closest to v1)
	hits, err := store.Search(ctx, vec3, 2, 0.0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "v1" {
		t.Errorf("expected top hit v1, got %s", hits[0].ID)
	}
	if hits[0].Score <= hits[1].Score {
		t.Errorf("hits not sorted by score descending: %f > %f expected", hits[0].Score, hits[1].Score)
	}

	// Search with minScore filter
	hits, err = store.Search(ctx, vec3, 10, 0.99)
	if err != nil {
		t.Fatalf("Search with minScore: %v", err)
	}
	for _, h := range hits {
		if h.Score < 0.99 {
			t.Errorf("hit %s score %f below minScore 0.99", h.ID, h.Score)
		}
	}

	// Delete v1 and verify
	if err := store.Delete(ctx, "v1"); err != nil {
		t.Fatalf("Delete v1: %v", err)
	}
	hits, err = store.Search(ctx, vec1, 10, 0.0)
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	for _, h := range hits {
		if h.ID == "v1" {
			t.Error("v1 should have been deleted")
		}
	}
}

func TestSQLiteVectorStore_UpsertOverwrite(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store, err := NewSQLiteVectorStore(db, "test_overwrite", nil)
	if err != nil {
		t.Fatalf("NewSQLiteVectorStore: %v", err)
	}
	ctx := context.Background()

	vec1 := []float64{1.0, 0.0}
	vec2 := []float64{0.0, 1.0}

	if err := store.Upsert(ctx, "v1", vec1, map[string]string{"k": "v1"}); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	// Overwrite with different vector
	if err := store.Upsert(ctx, "v1", vec2, map[string]string{"k": "v2"}); err != nil {
		t.Fatalf("Upsert v1 overwrite: %v", err)
	}

	hits, err := store.Search(ctx, vec2, 1, 0.99)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "v1" {
		t.Errorf("expected 1 hit with id v1, got %v", hits)
	}
	if hits[0].Meta["k"] != "v2" {
		t.Errorf("expected meta k=v2, got %s", hits[0].Meta["k"])
	}
}

func TestCosineSim64(t *testing.T) {
	tests := []struct {
		a, b   []float64
		expect float64
	}{
		{[]float64{1, 0, 0}, []float64{1, 0, 0}, 1.0},
		{[]float64{1, 0, 0}, []float64{0, 1, 0}, 0.0},
		{[]float64{1, 1, 0}, []float64{1, 0, 0}, 1.0 / math.Sqrt(2)},
		{[]float64{}, []float64{1}, 0.0},
		{[]float64{1, 2}, []float64{3, 4}, 11.0 / (math.Sqrt(5) * math.Sqrt(25))},
	}
	for _, tt := range tests {
		got := cosineSim64(tt.a, tt.b)
		if math.Abs(got-tt.expect) > 1e-9 {
			t.Errorf("cosineSim64(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.expect)
		}
	}
}
