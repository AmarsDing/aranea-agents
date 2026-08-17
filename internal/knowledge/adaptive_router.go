package knowledge

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
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
	// 自治理图谱 M1-2：检索命中日志 + base-level 激活分加成（可选；nil = 纯检索分）。
	accessLog bizknowledge.AccessLogRepo
	beta      float64
	// 自治理图谱 M1-3：Hebbian 共激活边（可选；nil = 不写）。异步触发，不阻塞返回。
	coact bizknowledge.CoActivationRepo
	eta   float64
	lg    loggateway.Logger
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

// SetAccessLog 接线检索命中日志（自治理图谱 M1-2，可选；nil = 不加成不记录）。
// beta 为 base-level 激活分加成系数（doc 默认 0.1）。最终分 = 检索分 + beta*baseLevel(doc)。
func (a *AdaptiveRouter) SetAccessLog(repo bizknowledge.AccessLogRepo, beta float64) {
	if a == nil {
		return
	}
	a.accessLog = repo
	a.beta = beta
}

// SetCoActivation 接线 Hebbian 共激活边（自治理图谱 M1-3，可选；nil = 不写）。
// eta 为单次共激活强化步长（doc 默认 0.1）；周期衰减由 dream_cycle 负责（M4）。
func (a *AdaptiveRouter) SetCoActivation(repo bizknowledge.CoActivationRepo, eta float64) {
	if a == nil {
		return
	}
	a.coact = repo
	a.eta = eta
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
		mode = a.selectModeForQuery(q, complexity)
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
	if a.accessLog != nil && len(chunks) > 0 {
		chunks = a.applyBaseLevelBoost(ctx, q, chunks)
	}
	if a.coact != nil && len(chunks) > 1 {
		a.triggerHebbian(ctx, q, chunks)
	}
	return chunks, nil
}

// triggerHebbian 异步强化同批召回文档的共激活边（M1-3）。脱离请求 ctx：
// 返回后请求取消不得中断写入（后台派生副作用，失败仅告警）。
func (a *AdaptiveRouter) triggerHebbian(ctx context.Context, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk) {
	docSet := make(map[string]struct{}, len(chunks))
	docIDs := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		if _, ok := docSet[ch.DocID]; !ok {
			docSet[ch.DocID] = struct{}{}
			docIDs = append(docIDs, ch.DocID)
		}
	}
	bg := context.WithoutCancel(ctx)
	collectionID := q.CollectionID
	safego.Go(bg, "knowledge.hebbian", func() {
		if err := a.coact.StrengthenCoActivations(bg, collectionID, docIDs, a.eta); err != nil {
			a.lg.Warn("Hebbian 共激活边写入失败",
				loggateway.StepID("knowledge.hebbian.fail"),
				loggateway.Str("collection_id", collectionID),
				loggateway.Err(err))
		}
	})
}

// applyBaseLevelBoost 用历史命中计算 base-level 激活分并加成到最终排序（M1-2）。
// 顺序：先用历史加成排序，再记录本次命中——本次检索不得加成自身（防循环自激）。
// 日志失败仅告警，不阻断检索返回。
func (a *AdaptiveRouter) applyBaseLevelBoost(ctx context.Context, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	docSet := make(map[string]struct{}, len(chunks))
	docIDs := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		if _, ok := docSet[ch.DocID]; !ok {
			docSet[ch.DocID] = struct{}{}
			docIDs = append(docIDs, ch.DocID)
		}
	}
	if scores, err := a.accessLog.BaseLevelScores(ctx, q.CollectionID, docIDs); err != nil {
		a.lg.Warn("base-level 激活分查询失败，跳过加成",
			loggateway.StepID("knowledge.access_boost.score_fail"),
			loggateway.Str("collection_id", q.CollectionID),
			loggateway.Err(err))
	} else if len(scores) > 0 {
		for i := range chunks {
			chunks[i].Score += float32(a.beta * scores[chunks[i].DocID])
		}
		sortChunksByScoreDesc(chunks)
	}
	entries := accessEntriesFromChunks(q.CollectionID, q.Query, chunks)
	if err := a.accessLog.LogAccess(ctx, entries); err != nil {
		a.lg.Warn("检索命中日志写入失败",
			loggateway.StepID("knowledge.access_boost.log_fail"),
			loggateway.Str("collection_id", q.CollectionID),
			loggateway.Err(err))
	}
	return chunks
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

var (
	exactIdentifierQueryRe = regexp.MustCompile(`(?i)^[a-z0-9]+(?:[-_.:][a-z0-9]+)+$`)
	uppercaseTokenQueryRe  = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,}$`)
)

func (a *AdaptiveRouter) selectModeForQuery(q biz.KnowledgeSearchQuery, complexity QueryComplexity) HybridSearchMode {
	query := strings.TrimSpace(q.Query)
	if ClassifySearchIntent(query) == IntentInstant ||
		exactIdentifierQueryRe.MatchString(query) ||
		uppercaseTokenQueryRe.MatchString(query) {
		return HybridSparse
	}
	// 问句与长中文陈述走 RRF：dense  alone 会绕开 SearchChunksBM25 的内容针。
	// 精确词/路径仍走 sparse。不因此把 classify 抬到 Complex，避免默认 LLM MultiQuery。
	if ClassifySearchIntent(query) == IntentSemantic || bizknowledge.LooksLikeNaturalLanguageQuery(query) {
		return HybridRRF
	}
	return a.selectMode(complexity)
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
