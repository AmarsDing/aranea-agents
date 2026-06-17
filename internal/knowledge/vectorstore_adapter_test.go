package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore"
)

// stubVectorStore is a mock implementation of vector.VectorStore for testing.
type stubVectorStore struct {
	upserted []upsertCall
	deleted  []string
	searchFn func(ctx context.Context, embedding []float64, topK int, minScore float64) ([]vector.VectorHit, error)
}

type upsertCall struct {
	id        string
	embedding []float64
	meta      map[string]string
}

func (s *stubVectorStore) Upsert(_ context.Context, id string, embedding []float64, meta map[string]string) error {
	s.upserted = append(s.upserted, upsertCall{id: id, embedding: embedding, meta: meta})
	return nil
}

func (s *stubVectorStore) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]vector.VectorHit, error) {
	if s.searchFn != nil {
		return s.searchFn(ctx, embedding, topK, minScore)
	}
	return nil, nil
}

func (s *stubVectorStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func newTestAdapter(store vector.VectorStore) *VectorStoreAdapter {
	return NewVectorStoreAdapter(store, loggateway.NewNoop())
}

func TestVectorStoreAdapter_Add(t *testing.T) {
	store := &stubVectorStore{}
	adapter := newTestAdapter(store)

	doc := &document.Document{
		ID:   "doc-1",
		Name: "test doc",
		Metadata: map[string]any{
			"source": "unit-test",
			"page":   float64(3),
		},
	}
	embedding := []float64{0.1, 0.2, 0.3}

	if err := adapter.Add(context.Background(), doc, embedding); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(store.upserted))
	}
	call := store.upserted[0]
	if call.id != "doc-1" {
		t.Errorf("expected id doc-1, got %q", call.id)
	}
	if call.meta["source"] != "unit-test" {
		t.Errorf("expected meta source=unit-test, got %q", call.meta["source"])
	}
	// float64 value becomes JSON string "3" via json.Marshal
	if call.meta["page"] != "3" {
		t.Errorf("expected meta page=3, got %q", call.meta["page"])
	}
}

func TestVectorStoreAdapter_Update(t *testing.T) {
	store := &stubVectorStore{}
	adapter := newTestAdapter(store)

	doc := &document.Document{
		ID:       "doc-2",
		Metadata: map[string]any{"key": "value"},
	}
	embedding := []float64{0.5}

	if err := adapter.Update(context.Background(), doc, embedding); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(store.upserted))
	}
	if store.upserted[0].id != "doc-2" {
		t.Errorf("expected id doc-2, got %q", store.upserted[0].id)
	}
}

func TestVectorStoreAdapter_Delete(t *testing.T) {
	store := &stubVectorStore{}
	adapter := newTestAdapter(store)

	if err := adapter.Delete(context.Background(), "doc-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if len(store.deleted) != 1 || store.deleted[0] != "doc-1" {
		t.Errorf("expected deleted [doc-1], got %v", store.deleted)
	}
}

func TestVectorStoreAdapter_Search(t *testing.T) {
	store := &stubVectorStore{
		searchFn: func(_ context.Context, _ []float64, topK int, _ float64) ([]vector.VectorHit, error) {
			if topK != 5 {
				t.Errorf("expected topK=5, got %d", topK)
			}
			return []vector.VectorHit{
				{ID: "hit-1", Score: 0.95, Meta: map[string]string{"source": "a"}},
				{ID: "hit-2", Score: 0.80, Meta: nil},
			}, nil
		},
	}
	adapter := newTestAdapter(store)

	query := &vectorstore.SearchQuery{
		Vector:   []float64{0.1, 0.2},
		Limit:    5,
		MinScore: 0.7,
	}
	result, err := adapter.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	r0 := result.Results[0]
	if r0.Document.ID != "hit-1" {
		t.Errorf("expected ID hit-1, got %q", r0.Document.ID)
	}
	if r0.Score != 0.95 {
		t.Errorf("expected Score 0.95, got %f", r0.Score)
	}
	if r0.Document.Metadata["source"] != "a" {
		t.Errorf("expected metadata source=a, got %v", r0.Document.Metadata["source"])
	}

	r1 := result.Results[1]
	if r1.Document.ID != "hit-2" {
		t.Errorf("expected ID hit-2, got %q", r1.Document.ID)
	}
}

func TestVectorStoreAdapter_Search_Error(t *testing.T) {
	wantErr := fmt.Errorf("search failed")
	store := &stubVectorStore{
		searchFn: func(_ context.Context, _ []float64, _ int, _ float64) ([]vector.VectorHit, error) {
			return nil, wantErr
		},
	}
	adapter := newTestAdapter(store)

	_, err := adapter.Search(context.Background(), &vectorstore.SearchQuery{
		Vector: []float64{0.1},
		Limit:  1,
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected search error, got %v", err)
	}
}

func TestVectorStoreAdapter_Get_Unsupported(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	doc, emb, err := adapter.Get(context.Background(), "any-id")
	if err == nil {
		t.Error("Get should return error for unsupported operation")
	}
	if doc != nil {
		t.Errorf("expected nil document, got %v", doc)
	}
	if emb != nil {
		t.Errorf("expected nil embedding, got %v", emb)
	}
}

func TestVectorStoreAdapter_DeleteByFilter_Unsupported(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	err := adapter.DeleteByFilter(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported DeleteByFilter, got nil")
	}
}

func TestVectorStoreAdapter_UpdateByFilter_Unsupported(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	_, err := adapter.UpdateByFilter(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported UpdateByFilter, got nil")
	}
}

func TestVectorStoreAdapter_Count_Unsupported(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	count, err := adapter.Count(context.Background())
	if err != nil {
		t.Errorf("Count should not return error, got %v", err)
	}
	if count != -1 {
		t.Errorf("expected -1, got %d", count)
	}
}

func TestVectorStoreAdapter_GetMetadata_Unsupported(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	meta, err := adapter.GetMetadata(context.Background())
	if err != nil {
		t.Errorf("GetMetadata should not return error, got %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata, got %v", meta)
	}
}

func TestVectorStoreAdapter_Close(t *testing.T) {
	adapter := newTestAdapter(&stubVectorStore{})

	if err := adapter.Close(); err != nil {
		t.Errorf("Close should return nil, got %v", err)
	}
}

func TestMetaAnyToString(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "nil map",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty map",
			input: map[string]any{},
			want:  nil,
		},
		{
			name:  "string values",
			input: map[string]any{"key": "value"},
			want:  map[string]string{"key": "value"},
		},
		{
			name:  "numeric value becomes string",
			input: map[string]any{"page": float64(3)},
			want:  map[string]string{"page": "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metaAnyToString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("metaAnyToString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("metaAnyToString()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestMetaStringToAny(t *testing.T) {
	input := map[string]string{"source": "test", "page": "3"}
	got := metaStringToAny(input)

	if got["source"] != "test" {
		t.Errorf("expected source=test, got %v", got["source"])
	}
	if got["page"] != "3" {
		t.Errorf("expected page=3, got %v", got["page"])
	}

	// nil input
	if metaStringToAny(nil) != nil {
		t.Error("expected nil for nil input")
	}
}
