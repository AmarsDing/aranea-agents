package knowledge

import (
	"context"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type QueryComplexity int

const (
	QuerySimple QueryComplexity = iota
	QueryModerate
	QueryComplex
)

type AdaptiveRouter struct {
	hybrid   *HybridRetriever
	rewriter *QueryRewriter
	expander *GraphExpander
	lg       loggateway.Logger
}

func NewAdaptiveRouter(hybrid *HybridRetriever, rewriter *QueryRewriter, lg loggateway.Logger) *AdaptiveRouter {
	return &AdaptiveRouter{hybrid: hybrid, rewriter: rewriter, lg: lg}
}

// SetGraphExpander 接线 Lazy GraphRAG 一跳扩展（可选；nil = 不扩展）。
func (a *AdaptiveRouter) SetGraphExpander(expander *GraphExpander) {
	if a == nil {
		return
	}
	a.expander = expander
}

func (a *AdaptiveRouter) Hybrid() *HybridRetriever {
	return a.hybrid
}

func (a *AdaptiveRouter) QueryRewriter() *QueryRewriter {
	return a.rewriter
}

func (a *AdaptiveRouter) Search(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	if a.hybrid == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "adaptive_router: hybrid retriever not configured")
	}

	complexity := a.classify(q, rewriteResult)
	var mode HybridSearchMode
	if modeOverride != "" && modeOverride != HybridAuto {
		mode = modeOverride
	} else {
		mode = a.selectMode(complexity)
	}

	used := RewriteNone
	if rewriteResult != nil {
		used = rewriteResult.Used
	}
	if auto := pickAutoRewriteStrategy(complexity, used); auto != RewriteNone && a.rewriter != nil {
		rr, err := a.rewriter.Rewrite(ctx, q.Query, auto)
		if err != nil {
			a.lg.Warn("复杂查询自动重写失败，使用原查询",
				loggateway.StepID("knowledge.adaptive.auto_rewrite_fail"),
				loggateway.Err(err))
		} else if rr != nil {
			rewriteResult = rr
		}
	}

	var chunks []biz.KnowledgeChunk
	var err error
	if rewriteResult != nil && len(rewriteResult.Queries) > 1 {
		chunks, err = a.searchMultiQuery(ctx, q, rewriteResult, mode)
	} else {
		chunks, err = a.hybrid.Search(ctx, q, mode)
	}
	if err != nil {
		return nil, err
	}
	if a.expander != nil {
		chunks = a.expander.Expand(ctx, q, chunks)
	}
	return chunks, nil
}

// pickAutoRewriteStrategy 仅在调用方未指定策略且查询判定为复杂时启用 MultiQuery。
// 简单/中等查询不加 LLM 往返，避免把 Advanced RAG 做成默认税。
func pickAutoRewriteStrategy(complexity QueryComplexity, already RewriteStrategy) RewriteStrategy {
	if already != RewriteNone {
		return RewriteNone
	}
	if complexity == QueryComplex {
		return RewriteMultiQuery
	}
	return RewriteNone
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
	failCount := 0
	for _, subQ := range queries {
		searchQ := q
		searchQ.Query = subQ
		searchQ.TopK = perQueryTopK
		chunks, err := a.hybrid.Search(ctx, searchQ, mode)
		if err != nil {
			failCount++
			a.lg.Warn("子查询检索失败",
				loggateway.StepID("knowledge.adaptive.sub_query_fail"),
				loggateway.Str("query", subQ),
				loggateway.Err(err))
			continue
		}
		allChunks = append(allChunks, chunks)
	}

	if failCount == len(queries) && len(queries) > 0 {
		a.lg.Warn("所有子查询均检索失败",
			loggateway.StepID("knowledge.adaptive.all_sub_query_fail"),
			loggateway.Str("original_query", q.Query),
			loggateway.Int("sub_query_count", len(queries)))
		return nil, apierror.Internal(apierror.DomainKnowledge,
			"adaptive_router: all sub-queries failed")
	}

	return mergeMultiQueryResults(allChunks, topK), nil
}
