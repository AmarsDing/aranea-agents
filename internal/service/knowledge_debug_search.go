package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
)

// DebugKnowledgeSearch（G-19 hit-test）：单库检索调试端点——复用 Search 的
// 路由/混合检索/rerank 全链路（排序保真，base-level 历史加成照常），差异：
//   - collection_id 必填（面板按库排查）；
//   - 不跑 RetrievalEvaluator（省 ~1.4s LLM，调试不一致性噪声）；
//   - 不落 access_log（SkipAccessLog：调试流量反复试探不得污染 base-level
//     激活分与个性化信号）。
//
// 每个 chunk 携带 G-12 分路分解（dense/sparse/rrf/rerank），调用方可用
// rrf_score vs score 分别还原重排前后序；mode_used/reranked 为服务端依据
// 分解实际产出推断的执行真值（embedder 故障降级 sparse 时如实呈现）。
func (s *KnowledgeService) DebugKnowledgeSearch(ctx context.Context, req *v1.DebugKnowledgeSearchRequest) (*v1.DebugKnowledgeSearchResponse, error) {
	colID := strings.TrimSpace(req.GetCollectionId())
	if colID == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "collection_id is required")
	}
	// C-01：与 Search 一致的 IDOR 防护——存在性 + 读权限。
	col, err := s.uc.GetCollection(ctx, colID)
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "query is required")
	}
	if s.search.Retriever == nil {
		return nil, apierror.Unavailable("KNOWLEDGE", "knowledge retriever not configured")
	}

	q := biz.KnowledgeSearchQuery{
		CollectionID:  colID,
		Query:         query,
		TopK:          int(req.GetTopK()),
		MinScore:      req.GetMinScore(),
		FilterJSON:    req.GetFilterJson(),
		PathPrefix:    strings.TrimSpace(req.GetPathPrefix()),
		SkipAccessLog: true,
	}
	if req.UseRerank != nil {
		v := req.GetUseRerank()
		q.UseRerank = &v
	}
	// hit-test 不带 rewrite 策略（空 = 不改写），面包屑由 Router/Hybrid 自行处理。
	rewriteResult := s.rewriteSearchQuery(ctx, query, "")
	modeOverride := knowledge.ParseHybridSearchMode(req.GetHybridSearch())

	var chunks []biz.KnowledgeChunk
	if s.search.Router != nil {
		chunks, err = s.search.Router.Search(ctx, q, rewriteResult, modeOverride)
	} else {
		chunks, err = s.search.Retriever.Search(ctx, q)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*v1.KnowledgeChunk, 0, len(chunks))
	for _, ch := range chunks {
		out = append(out, &v1.KnowledgeChunk{
			Id:            ch.ID,
			DocId:         ch.DocID,
			CollectionId:  ch.CollectionID,
			Content:       ch.Content,
			MetadataJson:  ch.MetadataJSON,
			ChunkIndex:    int32(ch.ChunkIndex),
			Score:         ch.Score,
			DenseScore:    ch.DenseScore,
			SparseScore:   ch.SparseScore,
			RrfScore:      ch.RRFScore,
			RerankScore:   ch.RerankScore,
			ParentContent: ch.ParentContent,
		})
	}
	return &v1.DebugKnowledgeSearchResponse{
		Chunks:   out,
		ModeUsed: inferDebugModeUsed(chunks),
		Reranked: debugReranked(chunks),
	}, nil
}

// inferDebugModeUsed 由分路分解推断实际执行的检索模式（请求给了 rrf 但
// embedder 故障降级时如实回 "sparse"）。排序模式对单次检索全局唯一，按序
// 命中首个非零分解即得。
func inferDebugModeUsed(chunks []biz.KnowledgeChunk) string {
	for _, ch := range chunks {
		if ch.RRFScore != 0 {
			return "rrf"
		}
		if ch.SparseScore != 0 {
			return "sparse"
		}
		if ch.DenseScore != 0 {
			return "dense"
		}
	}
	return "none"
}

func debugReranked(chunks []biz.KnowledgeChunk) bool {
	for _, ch := range chunks {
		if ch.RerankScore != 0 {
			return true
		}
	}
	return false
}
