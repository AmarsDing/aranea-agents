package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type FederationStrategy int

const (
	FederationBroadcast FederationStrategy = iota
	FederationRoute
)

type CollectionMetaFetcher interface {
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]biz.KnowledgeCollection, int, error)
}

type FederatedSearchOptions struct {
	Strategy       FederationStrategy
	RouteTopN      int
	RouteMinScore  float32
	EnumerateLimit int // SearchAll 枚举 Collection 上限（默认 64）
	BroadcastCap   int // 广播/降级扇出 Collection 上限（默认 8）
}

func DefaultFederatedSearchOptions() FederatedSearchOptions {
	return FederatedSearchOptions{
		Strategy:       FederationBroadcast,
		RouteTopN:      3,
		RouteMinScore:  0.3,
		EnumerateLimit: 64,
		BroadcastCap:   8,
	}
}

func (o FederatedSearchOptions) enumerateLimit() int {
	if o.EnumerateLimit <= 0 {
		return 64
	}
	return o.EnumerateLimit
}

func (o FederatedSearchOptions) broadcastCap() int {
	if o.BroadcastCap <= 0 {
		return 8
	}
	return o.BroadcastCap
}

type FederatedRetriever struct {
	router    *AdaptiveRouter
	retriever *Retriever
	meta      CollectionMetaFetcher
	lg        loggateway.Logger
}

func NewFederatedRetriever(router *AdaptiveRouter, retriever *Retriever, lg loggateway.Logger) *FederatedRetriever {
	return &FederatedRetriever{router: router, retriever: retriever, lg: lg}
}

func NewFederatedRetrieverWithMeta(router *AdaptiveRouter, retriever *Retriever, meta CollectionMetaFetcher, lg loggateway.Logger) *FederatedRetriever {
	return &FederatedRetriever{router: router, retriever: retriever, meta: meta, lg: lg}
}

func (f *FederatedRetriever) Search(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	if len(collectionIDs) == 0 {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "federated_retriever: at least one collection_id required")
	}
	if len(collectionIDs) == 1 {
		q.CollectionID = collectionIDs[0]
		if f.router != nil {
			return f.router.Search(ctx, q, rewriteResult, modeOverride)
		}
		return f.retriever.Search(ctx, q)
	}

	opts := DefaultFederatedSearchOptions()
	return f.searchBroadcast(ctx, capCollectionIDs(collectionIDs, opts.broadcastCap()), q, rewriteResult, modeOverride)
}

// SearchAll 全库智能路由（US-14 检索免选择）：枚举 Collection → Route 策略
// （名称/描述与 query 匹配度取 top N，阈值 0.3；无匹配自动降级 Broadcast）。
// workspace 为租户过滤键（C-01）：""=system 见全部，非空=租户自有+共享集合。
// 系统无任何可见 Collection 时返回空结果而非错误——LLM 可继续无知识回答，不阻塞会话。
func (f *FederatedRetriever) SearchAll(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode, workspace string) ([]biz.KnowledgeChunk, error) {
	if f.meta == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "federated_retriever: collection meta not configured")
	}
	opts := DefaultFederatedSearchOptions()
	opts.Strategy = FederationRoute
	cols, _, err := f.meta.ListCollections(ctx, workspace, opts.enumerateLimit(), 0)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	ids := make([]string, 0, len(cols))
	for _, c := range cols {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// 扇出前重写一次（显式策略由调用方传入；空则复杂查询自动 MultiQuery）。
	rewriteResult = f.prepareRewriteOnce(ctx, q, rewriteResult)
	routed := rankCollections(cols, ids, q.Query, opts)
	targets := routed
	if len(targets) == 0 {
		targets = capCollectionIDs(ids, opts.broadcastCap())
	} else {
		targets = capCollectionIDs(targets, opts.broadcastCap())
	}
	// 已在本函数完成 Route，禁止 SearchWithOptions 再 List+Route。
	opts.Strategy = FederationBroadcast
	return f.SearchWithOptions(ctx, targets, q, rewriteResult, modeOverride, opts)
}

func (f *FederatedRetriever) SearchWithOptions(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode, opts FederatedSearchOptions) ([]biz.KnowledgeChunk, error) {
	if len(collectionIDs) == 0 {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "federated_retriever: at least one collection_id required")
	}
	if len(collectionIDs) == 1 {
		q.CollectionID = collectionIDs[0]
		if f.router != nil {
			return f.router.Search(ctx, q, rewriteResult, modeOverride)
		}
		return f.retriever.Search(ctx, q)
	}

	if opts.Strategy == FederationRoute && f.meta != nil {
		routed, err := f.routeCollections(ctx, collectionIDs, q.Query, opts)
		if err != nil {
			f.lg.Warn("路由策略失败，降级广播",
				loggateway.StepID("knowledge.federated.route_fail"),
				loggateway.Err(err))
		} else if len(routed) > 0 {
			return f.searchBroadcast(ctx, capCollectionIDs(routed, opts.broadcastCap()), q, rewriteResult, modeOverride)
		}
	}

	return f.searchBroadcast(ctx, capCollectionIDs(collectionIDs, opts.broadcastCap()), q, rewriteResult, modeOverride)
}

func (f *FederatedRetriever) prepareRewriteOnce(ctx context.Context, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult) *QueryRewriteResult {
	if rewriteResult != nil || f.router == nil {
		return rewriteResult
	}
	return f.router.PrepareRewrite(ctx, q, nil)
}

func (f *FederatedRetriever) routeCollections(ctx context.Context, collectionIDs []string, query string, opts FederatedSearchOptions) ([]string, error) {
	collections, _, err := f.meta.ListCollections(ctx, "", opts.enumerateLimit(), 0)
	if err != nil {
		return nil, err
	}
	routed := rankCollections(collections, collectionIDs, query, opts)
	if len(routed) == 0 {
		return collectionIDs, nil
	}
	return routed, nil
}

func rankCollections(collections []biz.KnowledgeCollection, collectionIDs []string, query string, opts FederatedSearchOptions) []string {
	idSet := make(map[string]struct{}, len(collectionIDs))
	for _, id := range collectionIDs {
		idSet[id] = struct{}{}
	}

	type scored struct {
		id    string
		score float32
	}
	var ranked []scored
	queryLower := strings.ToLower(query)
	queryTerms := splitTerms(queryLower)

	for _, col := range collections {
		if _, ok := idSet[col.ID]; !ok {
			continue
		}
		s := collectionRelevanceScore(col, queryLower, queryTerms)
		if s >= opts.RouteMinScore {
			ranked = append(ranked, scored{id: col.ID, score: s})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	topN := opts.RouteTopN
	if topN <= 0 {
		topN = 3
	}
	if topN > len(ranked) {
		topN = len(ranked)
	}

	result := make([]string, 0, topN)
	for i := 0; i < topN; i++ {
		result = append(result, ranked[i].id)
	}
	return result
}

func capCollectionIDs(ids []string, limit int) []string {
	if limit <= 0 || len(ids) <= limit {
		return ids
	}
	return ids[:limit]
}

func collectionRelevanceScore(col biz.KnowledgeCollection, queryLower string, queryTerms []string) float32 {
	var score float32

	nameLower := strings.ToLower(col.Name)
	descLower := strings.ToLower(col.Description)

	for _, term := range queryTerms {
		if strings.Contains(nameLower, term) {
			score += 0.4
		}
		if strings.Contains(descLower, term) {
			score += 0.2
		}
	}

	if strings.Contains(nameLower, queryLower) {
		score += 0.5
	}
	if strings.Contains(descLower, queryLower) {
		score += 0.3
	}

	return score
}

func splitTerms(s string) []string {
	var terms []string
	for _, w := range strings.Fields(s) {
		w = strings.TrimSpace(w)
		if w != "" {
			terms = append(terms, w)
		}
	}
	return terms
}

func (f *FederatedRetriever) searchBroadcast(ctx context.Context, collectionIDs []string, q biz.KnowledgeSearchQuery, rewriteResult *QueryRewriteResult, modeOverride HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	type collResult struct {
		chunks []biz.KnowledgeChunk
		err    error
	}

	results := make([]collResult, len(collectionIDs))
	var wg sync.WaitGroup
	wg.Add(len(collectionIDs))

	for i, cid := range collectionIDs {
		idx := i
		collQ := q
		collQ.CollectionID = cid
		safego.Go(ctx, "knowledge-federated-search", func() {
			defer wg.Done()
			// Set default error in case of panic (safego recovers panics,
			// but results[idx] would otherwise remain zero-valued {nil, nil},
			// causing silent result loss).
			results[idx] = collResult{err: apierror.Internal(apierror.DomainKnowledge,
				fmt.Sprintf("federated search: collection %s panicked or did not complete", collectionIDs[idx]))}
			if f.router != nil {
				chunks, err := f.router.Search(ctx, collQ, rewriteResult, modeOverride)
				results[idx] = collResult{chunks: chunks, err: err}
				return
			}
			chunks, err := f.retriever.Search(ctx, collQ)
			results[idx] = collResult{chunks: chunks, err: err}
		})
	}

	wg.Wait()

	var allChunks []biz.KnowledgeChunk
	var firstErr error
	for i, r := range results {
		if r.err != nil {
			f.lg.Warn(fmt.Sprintf("collection %s search failed", collectionIDs[i]),
				loggateway.StepID("knowledge.federated_retriever"),
				loggateway.Err(r.err))
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		allChunks = append(allChunks, r.chunks...)
	}

	if len(allChunks) == 0 && firstErr != nil {
		return nil, apierror.Internal(apierror.DomainKnowledge, "federated_retriever: all collections failed").WithCause(firstErr)
	}

	return MergeSearchResults(nil, allChunks, q.TopK), nil
}
