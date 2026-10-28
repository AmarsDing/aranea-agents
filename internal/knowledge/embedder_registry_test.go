package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type fixedDimEmbedder struct{ dim int }

func (e fixedDimEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return make([]float32, e.dim), nil
}

type dimKeyedRepo struct {
	stubKnowledgeRepo
	dims []int
}

func (r *dimKeyedRepo) SearchChunks(_ context.Context, _ biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	r.dims = append(r.dims, len(vec))
	return []biz.KnowledgeChunk{{ID: "dim-" + itoa(len(vec)), DocID: "d", Score: 0.8}}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestHybridDense_FanoutAcrossDims(t *testing.T) {
	repo := &dimKeyedRepo{}
	ret := NewRetriever(fixedDimEmbedder{dim: 3}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, nil, loggateway.NewNoop())
	h.SetExtraDimEmbedders([]QueryEmbedder{fixedDimEmbedder{dim: 4}})

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "值班制度", TopK: 5,
	}, HybridDense)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
	}
	if !ids["dim-3"] || !ids["dim-4"] {
		t.Fatalf("cross-dim dense hits = %+v, want dim-3 and dim-4", got)
	}
}
