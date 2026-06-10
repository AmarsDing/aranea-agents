package knowledge

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type emptyErrEmbedder struct{}

func (emptyErrEmbedder) EmbedSingle(_ context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("cannot embed empty query")
	}
	return []float32{0.1, 0.2}, nil
}

func TestRetriever_Search_EmptyQuery(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: nil}
	ret := NewRetriever(emptyErrEmbedder{}, repo, nil, loggateway.Global())

	_, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col-1",
		Query:        "",
		TopK:         5,
	})
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	ke := kerrors.FromError(err)
	if ke.Reason != "KNOWLEDGE" {
		t.Fatalf("expected reason KNOWLEDGE, got %q", ke.Reason)
	}
}

func TestRetriever_Search_InvalidCollectionID(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: nil}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.Global())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "nonexistent-col",
		Query:        "test query",
		TopK:         5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 chunks for invalid collection ID, got %d", len(out))
	}
}
