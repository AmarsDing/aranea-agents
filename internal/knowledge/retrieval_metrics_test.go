package knowledge

import (
	"math"
	"testing"

	"aranea-agents/internal/biz"
)

func TestEvaluateRetrievalMetrics(t *testing.T) {
	cases := []RetrievalGoldCase{
		{ID: "q1", RelevantDocIDs: []string{"a", "b"}},
		{ID: "q2", RelevantDocIDs: []string{"c"}},
		{ID: "q3", Abstain: true},
		{ID: "q4", Abstain: true},
	}
	results := map[string][]biz.KnowledgeChunk{
		"q1": {{DocID: "x"}, {DocID: "a"}, {DocID: "b"}},
		"q2": {{DocID: "c"}},
		"q3": nil,
		"q4": {{DocID: "noise"}},
	}

	got := EvaluateRetrievalMetrics(cases, results, []int{1, 2, 3})
	assertMetricClose(t, got.RecallAt[1], 0.5)
	assertMetricClose(t, got.RecallAt[2], 0.75)
	assertMetricClose(t, got.RecallAt[3], 1)
	assertMetricClose(t, got.HitRateAt[1], 0.5)
	assertMetricClose(t, got.HitRateAt[2], 1)
	assertMetricClose(t, got.MRR, 0.75)
	assertMetricClose(t, got.AbstentionAccuracy, 0.5)
	if got.RankedCases != 2 || got.AbstentionCases != 2 {
		t.Fatalf("case counts = ranked:%d abstain:%d", got.RankedCases, got.AbstentionCases)
	}
	if got.NDCGAt[3] <= got.NDCGAt[1] {
		t.Fatalf("nDCG@3=%v should exceed nDCG@1=%v", got.NDCGAt[3], got.NDCGAt[1])
	}
}

func TestEvaluateRetrievalMetrics_DeduplicatesChunksByDocument(t *testing.T) {
	got := EvaluateRetrievalMetrics(
		[]RetrievalGoldCase{{ID: "q", RelevantDocIDs: []string{"a", "b"}}},
		map[string][]biz.KnowledgeChunk{"q": {
			{ID: "a1", DocID: "a"},
			{ID: "a2", DocID: "a"},
			{ID: "b1", DocID: "b"},
		}},
		[]int{2},
	)
	assertMetricClose(t, got.RecallAt[2], 1)
}

func assertMetricClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("metric = %v, want %v", got, want)
	}
}
