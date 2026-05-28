package knowledge

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestClassifyQueryComplexity(t *testing.T) {
	router := &AdaptiveRouter{}

	tests := []struct {
		name  string
		query string
		want  QueryComplexity
	}{
		{"empty", "", QuerySimple},
		{"short", "什么是Go", QuerySimple},
		{"moderate", "Go语言和Python语言的主要区别是什么", QueryModerate},
		{"complex", "请详细对比Go语言和Rust语言在并发编程方面的优缺点，以及它们在微服务架构中的适用场景", QueryComplex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := biz.KnowledgeSearchQuery{Query: tt.query}
			got := router.classify(q, nil)
			if got != tt.want {
				t.Errorf("classify(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSelectMode(t *testing.T) {
	router := &AdaptiveRouter{}
	tests := []struct {
		complexity QueryComplexity
		want       HybridSearchMode
	}{
		{QuerySimple, HybridDense},
		{QueryModerate, HybridRRF},
		{QueryComplex, HybridRRF},
	}
	for _, tt := range tests {
		got := router.selectMode(tt.complexity)
		if got != tt.want {
			t.Errorf("selectMode(%v) = %q, want %q", tt.complexity, got, tt.want)
		}
	}
}

func TestDecompositionBoostsComplexity(t *testing.T) {
	router := &AdaptiveRouter{}
	q := biz.KnowledgeSearchQuery{Query: "什么是Go"}
	rewriteResult := &QueryRewriteResult{
		Used:    RewriteDecomposition,
		Queries: []string{"Go是什么", "Go的特点", "Go的用途"},
	}
	got := router.classify(q, rewriteResult)
	if got < QueryModerate {
		t.Errorf("decomposition rewrite should boost complexity, got %v", got)
	}
}
