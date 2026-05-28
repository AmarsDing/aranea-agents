package knowledge

import (
	"context"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
)

type QueryComplexity int

const (
	QuerySimple    QueryComplexity = iota
	QueryModerate
	QueryComplex
)

type AdaptiveRouter struct {
	hybrid   *HybridRetriever
	rewriter *QueryRewriter
}

func NewAdaptiveRouter(hybrid *HybridRetriever, rewriter *QueryRewriter) *AdaptiveRouter {
	return &AdaptiveRouter{hybrid: hybrid, rewriter: rewriter}
}

func (a *AdaptiveRouter) Hybrid() *HybridRetriever {
	return a.hybrid
}

func (a *AdaptiveRouter) QueryRewriter() *QueryRewriter {
	return a.rewriter
}

func (a *AdaptiveRouter) Search(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult) ([]biz.KnowledgeChunk, error) {
	complexity := a.classify(q, rewriteResult)
	mode := a.selectMode(complexity)

	if rewriteResult != nil && len(rewriteResult.Queries) > 1 {
		return a.searchMultiQuery(ctx, q, rewriteResult, mode)
	}

	return a.hybrid.Search(ctx, q, mode)
}

func (a *AdaptiveRouter) classify(q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult) QueryComplexity {
	query := strings.TrimSpace(q.Query)
	if query == "" {
		return QuerySimple
	}

	score := 0

	wordCount := 0
	cjkCount := 0
	for _, r := range query {
		if unicode.Is(unicode.Han, r) {
			cjkCount++
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			wordCount++
		}
	}
	effectiveWords := wordCount + cjkCount/2

	if effectiveWords > 20 {
		score += 2
	} else if effectiveWords > 10 {
		score += 1
	}

	questionMarks := strings.Count(query, "？") + strings.Count(query, "?")
	if questionMarks > 1 {
		score += 1
	}

	connectorWords := []string{"和", "与", "以及", "同时", "并且", "而且", "另外", "此外", "对比", "比较", "区别", "关系"}
	for _, w := range connectorWords {
		if strings.Contains(query, w) {
			score += 1
			break
		}
	}

	if rewriteResult != nil && rewriteResult.Used == RewriteDecomposition {
		score += 2
	}

	if q.TopK > 10 {
		score += 1
	}

	switch {
	case score >= 3:
		return QueryComplex
	case score >= 1:
		return QueryModerate
	default:
		return QuerySimple
	}
}

func (a *AdaptiveRouter) selectMode(complexity QueryComplexity) HybridSearchMode {
	switch complexity {
	case QuerySimple:
		return HybridDense
	case QueryModerate:
		return HybridRRF
	case QueryComplex:
		return HybridRRF
	default:
		return HybridDense
	}
}

func (a *AdaptiveRouter) searchMultiQuery(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, mode HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	queries := dedupQueries(rewriteResult.Queries)
	topK := q.TopK
	if topK <= 0 {
		topK = 5
	}

	perQueryTopK := topK
	if len(queries) > 1 {
		perQueryTopK = topK * 2
		if perQueryTopK > 30 {
			perQueryTopK = 30
		}
	}

	allChunks := make([][]biz.KnowledgeChunk, 0, len(queries))
	for _, subQ := range queries {
		searchQ := q
		searchQ.Query = subQ
		searchQ.TopK = perQueryTopK
		chunks, err := a.hybrid.Search(ctx, searchQ, mode)
		if err != nil {
			continue
		}
		allChunks = append(allChunks, chunks)
	}

	return mergeMultiQueryResults(allChunks, topK), nil
}
