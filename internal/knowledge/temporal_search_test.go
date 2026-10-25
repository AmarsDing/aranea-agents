package knowledge

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// temporal_search_test.go G-32 T4 item 4 检索管线腿路由钉阵：
// stubKnowledgeRepo 时态读端口实现（既有 G-05 热度抑制用例经此真正走 as-of
// 腿——路由获得单元级实跑覆盖，行为断言原序不动）；未接线拒绝：span 非零而
// repo 未实现时态端口 → 结构性 Unavailable，不蜕化 current 快报。

var _ bizknowledge.ChunkTemporalReader = (*stubKnowledgeRepo)(nil)

func (s *stubKnowledgeRepo) asOfItems(topK int) []bizknowledge.KnowledgeChunkIncarnation {
	out := make([]bizknowledge.KnowledgeChunkIncarnation, 0, len(s.chunks))
	for _, ch := range s.chunks {
		out = append(out, bizknowledge.KnowledgeChunkIncarnation{
			ChunkID: ch.ID, DocID: ch.DocID, CollectionID: ch.CollectionID,
			Content: ch.Content, Score: float64(ch.Score),
		})
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}

func (s *stubKnowledgeRepo) ListChunksAsOf(_ context.Context, _ string, _ bizknowledge.TemporalSpan, _ string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	return s.asOfItems(0), nil
}

func (s *stubKnowledgeRepo) RecallChunksAsOf(_ context.Context, _ string, _ []float32, _ bizknowledge.TemporalSpan, topK int, _ string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	return s.asOfItems(topK), nil
}

func (s *stubKnowledgeRepo) SparseRecallChunksAsOf(_ context.Context, _, _ string, _ bizknowledge.TemporalSpan, topK int, _ string) ([]bizknowledge.KnowledgeChunkIncarnation, error) {
	return s.asOfItems(topK), nil
}

// nonTemporalRepo 类型擦除包装：方法集仅 biz.KnowledgeRepo（时态端口未接线形态）。
type nonTemporalRepo struct{ biz.KnowledgeRepo }

type stubSparseSearcher struct{}

func (stubSparseSearcher) SearchChunksBM25(context.Context, biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	return nil, nil
}

// 未接线拒绝：span 非零 + 端口缺席 → 结构性 Unavailable（dense/sparse 双腿
// 同口径；谎报时态等价不如拒绝）。
func TestSearchLegsAsOfNotWiredRefuses(t *testing.T) {
	span := bizknowledge.TemporalSpan{ValidAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	q := biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5, Temporal: span}

	r := NewRetriever(stubAllEmbedder{}, nonTemporalRepo{&stubKnowledgeRepo{}}, nil, loggateway.NewNoop())
	if _, err := r.searchDenseLeg(context.Background(), q, []float32{0.1}); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("retriever dense leg = %v, want Unavailable", err)
	}
	if _, err := r.searchSparseLeg(context.Background(), stubSparseSearcher{}, q); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("retriever sparse leg = %v, want Unavailable", err)
	}

	h := NewHybridRetriever(r, stubSparseSearcher{}, loggateway.NewNoop())
	if _, err := h.searchDenseLeg(context.Background(), q, []float32{0.1}); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("hybrid dense leg = %v, want Unavailable", err)
	}
	if _, err := h.searchSparseLeg(context.Background(), q); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("hybrid sparse leg = %v, want Unavailable", err)
	}
}

// 反向钉：span 零值 → 双腿逐字节委托原函数（G-05 条款①），端口实现不影响
// 非时态流量。
func TestSearchLegsZeroSpanDelegatesCurrentPath(t *testing.T) {
	spanless := biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}
	repo := &stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{{ID: "c1", Score: 1}}}
	r := NewRetriever(stubAllEmbedder{}, repo, nil, loggateway.NewNoop())
	out, err := r.searchDenseLeg(context.Background(), spanless, []float32{0.1})
	if err != nil {
		t.Fatalf("current-path dense leg: %v", err)
	}
	if len(out) != 1 || out[0].ID != "c1" {
		t.Fatalf("current-path must delegate verbatim, got %v", out)
	}
}
