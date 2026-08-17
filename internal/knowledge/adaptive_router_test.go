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

func TestSelectModeForQuery_PreservesExactLexicalSignals(t *testing.T) {
	router := &AdaptiveRouter{}
	for _, query := range []string{
		`"connection refused"`,
		`docs/runbook.md`,
		`ERR_AUTH_401`,
		`INC-2026-0817`,
	} {
		got := router.selectModeForQuery(biz.KnowledgeSearchQuery{Query: query}, QuerySimple)
		if got != HybridSparse {
			t.Errorf("selectModeForQuery(%q) = %q, want sparse", query, got)
		}
	}
	if got := router.selectModeForQuery(
		biz.KnowledgeSearchQuery{Query: "低压电工证"},
		QuerySimple,
	); got != HybridDense {
		t.Errorf("short keyword query = %q, want dense", got)
	}
}

func TestSelectModeForQuery_QuestionsUseRRF(t *testing.T) {
	router := &AdaptiveRouter{}
	for _, query := range []string{
		"什么是知识图谱",
		"核心机房的巡检周期是多久一次，周几几点开始？",
		"What is the inspection interval?",
	} {
		got := router.selectModeForQuery(biz.KnowledgeSearchQuery{Query: query}, QuerySimple)
		if got != HybridRRF {
			t.Errorf("selectModeForQuery(%q) = %q, want rrf", query, got)
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
