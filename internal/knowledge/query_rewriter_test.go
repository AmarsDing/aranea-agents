package knowledge

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseRewriteStrategy(t *testing.T) {
	tests := []struct {
		input string
		want  RewriteStrategy
	}{
		{"", RewriteNone},
		{"hyde", RewriteHyDE},
		{"HyDE", RewriteHyDE},
		{"decomposition", RewriteDecomposition},
		{"multi_query", RewriteMultiQuery},
		{"unknown", RewriteNone},
	}
	for _, tt := range tests {
		got := ParseRewriteStrategy(tt.input)
		if got != tt.want {
			t.Errorf("ParseRewriteStrategy(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDedupQueries(t *testing.T) {
	tests := []struct {
		name    string
		queries []string
		want    []string
	}{
		{"empty", []string{}, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"with_empty", []string{"a", "", "b"}, []string{"a", "b"}},
		{"with_spaces", []string{"a", " a ", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupQueries(tt.queries)
			if len(got) != len(tt.want) {
				t.Errorf("dedupQueries() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("dedupQueries()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMergeMultiQueryResults(t *testing.T) {
	chunks1 := []biz.KnowledgeChunk{
		{ID: "1", Content: "chunk1", Score: 0.9},
		{ID: "2", Content: "chunk2", Score: 0.7},
	}
	chunks2 := []biz.KnowledgeChunk{
		{ID: "2", Content: "chunk2", Score: 0.8},
		{ID: "3", Content: "chunk3", Score: 0.6},
	}

	merged := mergeMultiQueryResults([][]biz.KnowledgeChunk{chunks1, chunks2}, 5)
	if len(merged) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(merged))
	}
	if merged[0].ID != "1" {
		t.Errorf("expected highest score chunk '1', got '%s'", merged[0].ID)
	}
}

func TestMergeMultiQueryResultsTopK(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
		{ID: "3", Score: 0.5},
	}
	merged := mergeMultiQueryResults([][]biz.KnowledgeChunk{chunks}, 2)
	if len(merged) != 2 {
		t.Fatalf("expected 2 chunks with topK=2, got %d", len(merged))
	}
}

func TestStripCodeFenceJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `["a","b"]`, `["a","b"]`},
		{"with fence", "```json\n[\"a\",\"b\"]\n```", `["a","b"]`},
		{"with fence no lang", "```\n[\"a\",\"b\"]\n```", `["a","b"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFence(tt.input)
			if got != tt.want {
				t.Errorf("stripCodeFence() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFmtQueryRewriteResult(t *testing.T) {
	tests := []struct {
		name string
		r    *QueryRewriteResult
		want string
	}{
		{"nil", nil, ""},
		{"none", &QueryRewriteResult{Used: RewriteNone}, ""},
		{"hyde", &QueryRewriteResult{Used: RewriteHyDE, Queries: []string{"q1", "q2"}}, "strategy=hyde, queries=2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmtQueryRewriteResult(tt.r)
			if got != tt.want {
				t.Errorf("fmtQueryRewriteResult() = %q, want %q", got, tt.want)
			}
		})
	}
}
