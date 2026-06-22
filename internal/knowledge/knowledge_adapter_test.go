package knowledge

import (
	"context"
	"errors"
	"math"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/knowledge"
)

func TestKnowledgeAdapter_Search_ConvertsCorrectly(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{
			ID:           "chunk-1",
			DocID:        "doc-1",
			CollectionID: "col-1",
			ChunkIndex:   0,
			Content:      "Go is a statically typed language",
			Score:        0.95,
		},
		{
			ID:           "chunk-2",
			DocID:        "doc-1",
			CollectionID: "col-1",
			ChunkIndex:   1,
			Content:      "Go supports concurrency with goroutines",
			Score:        0.82,
		},
	}

	searchFunc := func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		if q.Query != "what is Go" {
			t.Errorf("expected query 'what is Go', got %q", q.Query)
		}
		if q.TopK != 10 {
			t.Errorf("expected TopK=10, got %d", q.TopK)
		}
		if q.CollectionID != "col-1" {
			t.Errorf("expected CollectionID='col-1', got %q", q.CollectionID)
		}
		return chunks, nil
	}

	adapter := NewKnowledgeAdapter(searchFunc, loggateway.NewNoop())

	req := &knowledge.SearchRequest{
		Query:      "what is Go",
		MaxResults: 10,
		MinScore:   0.5,
		SearchFilter: &knowledge.SearchFilter{
			Metadata: map[string]any{
				"collection_id": "col-1",
			},
		},
	}

	result, err := adapter.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if math.Abs(result.Score-0.95) > 1e-6 {
		t.Errorf("expected best score ~0.95, got %f", result.Score)
	}
	if result.Text != "Go is a statically typed language" {
		t.Errorf("expected best text from chunk-1, got %q", result.Text)
	}
	if result.Document == nil || result.Document.ID != "chunk-1" {
		t.Errorf("expected best document ID 'chunk-1', got %v", result.Document)
	}
	if len(result.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(result.Documents))
	}

	// Verify metadata mapping on first result.
	doc0 := result.Documents[0]
	if doc0.Document.Metadata["doc_id"] != "doc-1" {
		t.Errorf("expected metadata doc_id='doc-1', got %v", doc0.Document.Metadata["doc_id"])
	}
	if doc0.Document.Metadata["collection_id"] != "col-1" {
		t.Errorf("expected metadata collection_id='col-1', got %v", doc0.Document.Metadata["collection_id"])
	}
	if doc0.Document.Metadata["chunk_index"] != 0 {
		t.Errorf("expected metadata chunk_index=0, got %v", doc0.Document.Metadata["chunk_index"])
	}
	if math.Abs(doc0.Score-0.95) > 1e-6 {
		t.Errorf("expected score ~0.95, got %f", doc0.Score)
	}
}

func TestKnowledgeAdapter_Search_EmptyResults(t *testing.T) {
	searchFunc := func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		return nil, nil
	}

	adapter := NewKnowledgeAdapter(searchFunc, loggateway.NewNoop())

	req := &knowledge.SearchRequest{
		Query:      "nonexistent",
		MaxResults: 5,
	}

	result, err := adapter.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Document != nil {
		t.Errorf("expected nil Document for empty results, got %v", result.Document)
	}
	if result.Score != 0 {
		t.Errorf("expected zero Score for empty results, got %f", result.Score)
	}
	if result.Text != "" {
		t.Errorf("expected empty Text for empty results, got %q", result.Text)
	}
	if len(result.Documents) != 0 {
		t.Errorf("expected empty Documents, got %d", len(result.Documents))
	}
}

func TestKnowledgeAdapter_Search_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("database connection lost")

	searchFunc := func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		return nil, expectedErr
	}

	adapter := NewKnowledgeAdapter(searchFunc, loggateway.NewNoop())

	req := &knowledge.SearchRequest{
		Query:      "test query",
		MaxResults: 5,
	}

	result, err := adapter.Search(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr.Error(), err.Error())
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %v", result)
	}
}

func TestKnowledgeAdapter_Search_NilRequest(t *testing.T) {
	adapter := NewKnowledgeAdapter(nil, loggateway.NewNoop())

	result, err := adapter.Search(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for nil request")
	}
	if len(result.Documents) != 0 {
		t.Errorf("expected empty documents for nil request, got %d", len(result.Documents))
	}
}

func TestKnowledgeAdapter_Search_DefaultTopK(t *testing.T) {
	var capturedTopK int

	searchFunc := func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		capturedTopK = q.TopK
		return nil, nil
	}

	adapter := NewKnowledgeAdapter(searchFunc, loggateway.NewNoop())

	// MaxResults=0 should default to TopK=5.
	req := &knowledge.SearchRequest{
		Query:      "test",
		MaxResults: 0,
	}

	_, err := adapter.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTopK != 5 {
		t.Errorf("expected default TopK=5, got %d", capturedTopK)
	}
}

func TestKnowledgeAdapter_Search_MetadataJSONPreserved(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{
			ID:           "chunk-1",
			DocID:        "doc-1",
			CollectionID: "col-1",
			ChunkIndex:   0,
			Content:      "test content",
			Score:        0.9,
			MetadataJSON: `{"source":"pdf","page":3}`,
		},
	}

	searchFunc := func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		return chunks, nil
	}

	adapter := NewKnowledgeAdapter(searchFunc, loggateway.NewNoop())

	req := &knowledge.SearchRequest{
		Query:      "test",
		MaxResults: 5,
	}

	result, err := adapter.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := result.Documents[0].Document.Metadata
	if meta["source"] != "pdf" {
		t.Errorf("expected metadata source='pdf', got %v", meta["source"])
	}
	// JSON numbers decode as float64.
	if page, ok := meta["page"].(float64); !ok || page != 3 {
		t.Errorf("expected metadata page=3, got %v", meta["page"])
	}
	// Standard fields should also be present.
	if meta["doc_id"] != "doc-1" {
		t.Errorf("expected metadata doc_id='doc-1', got %v", meta["doc_id"])
	}
}
