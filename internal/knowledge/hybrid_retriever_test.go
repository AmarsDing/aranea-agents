package knowledge

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseHybridSearchMode(t *testing.T) {
	tests := []struct {
		input string
		want  HybridSearchMode
	}{
		{"", HybridAuto},
		{"auto", HybridAuto},
		{"dense", HybridDense},
		{"sparse", HybridSparse},
		{"rrf", HybridRRF},
		{"unknown", HybridAuto},
	}
	for _, tt := range tests {
		got := ParseHybridSearchMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseHybridSearchMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRRFMerge(t *testing.T) {
	dense := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 0.9},
		{ID: "b", Content: "beta", Score: 0.7},
		{ID: "c", Content: "gamma", Score: 0.5},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "b", Content: "beta", Score: 0.01},
		{ID: "d", Content: "delta", Score: 0.008},
		{ID: "a", Content: "alpha", Score: 0.005},
	}

	merged := rrfMerge(dense, sparse, 60)
	if len(merged) != 4 {
		t.Fatalf("expected 4 unique chunks, got %d", len(merged))
	}

	ids := make(map[string]bool)
	for _, ch := range merged {
		ids[ch.ID] = true
	}
	for _, want := range []string{"a", "b", "c", "d"} {
		if !ids[want] {
			t.Errorf("missing chunk %q in merged results", want)
		}
	}

	if merged[0].ID != "a" && merged[0].ID != "b" {
		t.Errorf("expected 'a' or 'b' to rank first (both appear in both lists), got %q", merged[0].ID)
	}
}

func TestMergeSearchResults(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
	}
	supplement := []biz.KnowledgeChunk{
		{ID: "2", Score: 0.8},
		{ID: "3", Score: 0.6},
	}

	merged := MergeSearchResults(primary, supplement, 5)
	if len(merged) != 3 {
		t.Fatalf("expected 3 unique chunks, got %d", len(merged))
	}
	if merged[0].Score < merged[1].Score {
		t.Errorf("results not sorted by score descending")
	}
}

func TestMergeSearchResultsTopK(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
		{ID: "3", Score: 0.5},
	}
	merged := MergeSearchResults(primary, nil, 2)
	if len(merged) != 2 {
		t.Fatalf("expected 2 chunks with topK=2, got %d", len(merged))
	}
}
