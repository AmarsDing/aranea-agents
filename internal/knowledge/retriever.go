// Package knowledge provides vector retrieval for knowledge base search.
package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
)

const defaultRerankOverfetchMin = 20
const maxRerankOverfetch = 50

// QueryEmbedder embeds search queries.
type QueryEmbedder interface {
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
}

// TaskTypeEmbedder extends QueryEmbedder with task-type-aware embedding.
// Providers like Gemini benefit from distinguishing RETRIEVAL_DOCUMENT vs RETRIEVAL_QUERY.
type TaskTypeEmbedder interface {
	QueryEmbedder
	EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error)
}

// Retriever orchestrates embedding + search (+ optional rerank) for a collection.
type Retriever struct {
	embedder QueryEmbedder
	repo     biz.KnowledgeRepo
	reranker reranker.Reranker
	lg       loggateway.Logger
	// monitorBus 流程日志总线（装配层经 SetMonitorBus 注入；nil 时跳过流程日志）。
	monitorBus contract.MonitorBus
	// accessLog 检索命中日志（P2-c 下沉，可选；nil = 不记录）。只记录不加成——
	// base-level 排序加成仍专属 AdaptiveRouter，本层不改排序语义。
	accessLog bizknowledge.AccessLogRepo
}

// NewRetriever creates a Retriever bound to the given embedder, repo, and optional reranker.
func NewRetriever(embedder QueryEmbedder, repo biz.KnowledgeRepo, rr reranker.Reranker, lg loggateway.Logger) *Retriever {
	return &Retriever{embedder: embedder, repo: repo, reranker: rr, lg: lg}
}

// SetMonitorBus 注入流程日志总线（装配层调用；nil = 不发射流程日志）。
func (r *Retriever) SetMonitorBus(bus contract.MonitorBus) {
	if r == nil {
		return
	}
	r.monitorBus = bus
}

// SetAccessLog 接线检索命中日志（P2-c 下沉，装配层调用；nil = 不记录）。
// 覆盖 Router 缺席/退化的全路径：service HTTP 直搜、联邦退化、cue 预检索退化、
// 框架原生 knowledge_search 退化。Router 在役路径由 Router 自记（HybridRetriever
// 直调 repo 不经本层），两路径互斥无双记。
func (r *Retriever) SetAccessLog(repo bizknowledge.AccessLogRepo) {
	if r == nil {
		return
	}
	r.accessLog = repo
}

// HasReranker reports whether a reranker is configured globally.
func (r *Retriever) HasReranker() bool {
	return r != nil && r.reranker != nil
}

// Search embeds the query, retrieves candidates, optionally reranks, and returns top-k chunks.
// 检索为热路径：流程日志只打 done/error 且 message 精简（不打 start、不写进程日志）。
func (r *Retriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	totalStart := time.Now()
	defer observeSearchStage(searchStageTotal, totalStart)
	chunks, err := r.search(ctx, q)
	if err == nil {
		r.logAccess(ctx, q, chunks, time.Since(totalStart))
	}
	if r != nil && r.monitorBus != nil {
		topK := q.TopK
		if topK <= 0 {
			topK = 5
		}
		flow := newKnowledgeFlow(ctx, r.monitorBus, nil)
		if err != nil {
			flow.LogError("knowledge.search", "知识库检索失败",
				event.P("collection_id", q.CollectionID),
				event.P("top_k", topK),
				event.P("error", err.Error()))
		} else {
			flow.LogDone("knowledge.search", "知识库检索完成",
				event.P("collection_id", q.CollectionID),
				event.P("top_k", topK),
				event.P("hits", len(chunks)))
		}
	}
	return chunks, err
}

// logAccess 异步记录本次命中（P2-c）。持久化延迟不进入检索热路径；
// 使用 WithoutCancel 保留 workspace/trace values，同时设置短超时避免后台泄漏。
// G-15：latency 为检索总时延（含量化与重排），随命中条目落库（审计足迹）。
func (r *Retriever) logAccess(ctx context.Context, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk, latency time.Duration) {
	// G-19：命中调试旁路不写热度日志（防调试流量污染 base-level 信号）。
	// G-05-asof：时态查询零热度读写（ADR-KN-20261020-J——Router 缺席/退化路径
	// 同样不得让回放查询污染当下 base-level/共激活信号）。
	if r == nil || r.accessLog == nil || len(chunks) == 0 || q.SkipAccessLog || q.Temporal.Any() {
		return
	}
	entries := accessEntriesFromChunks(q, newAccessContext(ctx, q, latency), chunks)
	if len(entries) == 0 {
		return
	}
	bg := context.WithoutCancel(ctx)
	safego.Go(bg, "knowledge.retriever.access_log", func() {
		writeCtx, cancel := context.WithTimeout(bg, 2*time.Second)
		defer cancel()
		if err := r.accessLog.LogAccess(writeCtx, entries); err != nil {
			r.lg.Warn("检索命中日志写入失败",
				loggateway.StepID("knowledge.retriever.access_log_fail"),
				loggateway.Str("collection_id", q.CollectionID),
				loggateway.Err(err))
		}
	})
}

// accessQueryHash 生成 access_log 的查询指纹（sha1 前 8 字节 hex）。
// Retriever/Router 共用，保证同一查询两路径同指纹。
func accessQueryHash(query string) string {
	sum := sha1.Sum([]byte(query))
	return hex.EncodeToString(sum[:8])
}

// accessContext （G-15 审计足迹）：命中日志的请求上下文透传——session/agent
// 由调用方经 SearchQuery 带入（Agent 工具回调 / cue 预检索），user 缺省时从
// 请求 ctx 的认证身份回填；latencyMs 为调用方计时的检索总时延。
type accessContext struct {
	sessionID string
	userID    string
	agentID   string
	latencyMs int64
}

func newAccessContext(ctx context.Context, q biz.KnowledgeSearchQuery, latency time.Duration) accessContext {
	userID := q.UserID
	if userID == "" {
		userID = ctxuser.FromContext(ctx)
	}
	return accessContext{
		sessionID: q.SessionID,
		userID:    userID,
		agentID:   q.AgentID,
		latencyMs: latency.Milliseconds(),
	}
}

// accessEntriesFromChunks 按 doc 去重构建命中日志条目（同批同 doc 多 chunk 记一次）。
// G-15：Rank/Score 取该文档首个 chunk 在最终排序中的位置（1-based）与最终排序分。
func accessEntriesFromChunks(q biz.KnowledgeSearchQuery, ids accessContext, chunks []biz.KnowledgeChunk) []bizknowledge.AccessLogEntry {
	docSet := make(map[string]struct{}, len(chunks))
	entries := make([]bizknowledge.AccessLogEntry, 0, len(chunks))
	for i, ch := range chunks {
		if ch.DocID == "" {
			continue
		}
		if _, ok := docSet[ch.DocID]; ok {
			continue
		}
		docSet[ch.DocID] = struct{}{}
		entries = append(entries, bizknowledge.AccessLogEntry{
			CollectionID: q.CollectionID,
			DocID:        ch.DocID,
			Rank:         i + 1,
			Score:        float64(ch.Score),
		})
	}
	if len(entries) == 0 {
		return nil
	}
	queryHash := accessQueryHash(q.Query)
	for i := range entries {
		entries[i].QueryHash = queryHash
		entries[i].SessionID = ids.sessionID
		entries[i].UserID = ids.userID
		entries[i].AgentID = ids.agentID
		entries[i].LatencyMs = ids.latencyMs
	}
	return entries
}

func (r *Retriever) search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if r.repo == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "retriever: repo is nil")
	}
	topK := q.TopK
	if topK <= 0 {
		topK = 5
	}

	lexical, err := collectionLacksSemanticLayer(ctx, r.repo, q.CollectionID)
	if err != nil {
		return nil, err
	}
	// F5：embedder 缺失或不可用（未配置）时降级 BM25 词法检索——V2 设计 embedding
	if r.embedder == nil {
		return r.searchSparseFallback(ctx, q, topK, "embedder is nil", nil)
	}
	// §V5 降级矩阵 #3：目标集合为无语义层词法库（embedding_model 空）时直接降级
	// BM25——chunks 无向量，dense 检索恒空。判定前置避免浪费一次查询 embed。
	if lexical {
		return r.searchSparseFallback(ctx, q, topK, "collection has no semantic layer", nil)
	}
	embedStart := time.Now()
	vec, err := r.embedQuery(ctx, q.Query)
	observeSearchStage(searchStageEmbed, embedStart)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeUnavailable) {
			return r.searchSparseFallback(ctx, q, topK, "embed failed", err)
		}
		return nil, apierror.Internal(apierror.DomainKnowledge, "retriever embed failed").WithCause(err)
	}

	searchQ := q
	searchQ.TopK = topK
	if r.shouldRerank(q) {
		searchQ.TopK = rerankCandidateLimit(q, topK)
	}

	denseStart := time.Now()
	chunks, err := r.searchDenseLeg(ctx, searchQ, vec)
	observeSearchStage(searchStageDense, denseStart)
	if err != nil {
		return nil, err
	}
	return r.RerankChunks(ctx, q, chunks, topK)
}

// searchSparseFallback 经 repo 的 BM25 能力降级检索。biz.Repo 接口刻意不含 BM25
// （SparseSearcher 独立），此处对 data 层具体实现做类型断言探测；repo 不支持 BM25
// 时保留原错误语义。
func (r *Retriever) searchSparseFallback(ctx context.Context, q biz.KnowledgeSearchQuery, topK int, reason string, cause error) ([]biz.KnowledgeChunk, error) {
	ss, ok := r.repo.(SparseSearcher)
	if !ok {
		if cause != nil {
			return nil, apierror.Internal(apierror.DomainKnowledge, "retriever embed failed").WithCause(cause)
		}
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "retriever: embedder is nil and repo has no BM25 fallback")
	}
	fields := []loggateway.Field{
		loggateway.StepID("knowledge.retriever.sparse_fallback"),
		loggateway.Str("collection_id", q.CollectionID),
		loggateway.Str("reason", reason),
	}
	if cause != nil {
		fields = append(fields, loggateway.Err(cause))
	}
	r.lg.Warn("语义层不可用，降级 BM25 词法检索", fields...)

	q.TopK = topK
	sparseStart := time.Now()
	chunks, err := r.searchSparseLeg(ctx, ss, q)
	observeSearchStage(searchStageSparse, sparseStart)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainKnowledge, "retriever sparse fallback failed").WithCause(err)
	}
	return trimChunks(chunks, topK), nil
}

// RerankChunks applies the reranker to pre-retrieved chunks and returns top-k results.
// If reranker is not configured or reranking fails, returns the original chunks trimmed to topK.
func (r *Retriever) RerankChunks(ctx context.Context, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk, topK int) ([]biz.KnowledgeChunk, error) {
	if !r.shouldRerank(q) || len(chunks) == 0 {
		return trimChunks(chunks, topK), nil
	}

	results := chunksToRerankerResults(chunks)
	reranked, err := r.reranker.Rerank(ctx, &reranker.Query{Text: q.Query, FinalQuery: q.Query}, results)
	if err != nil {
		r.lg.Warn("重排失败，使用向量排序",
			loggateway.StepID("knowledge.rerank.fallback"),
			loggateway.Err(err),
			loggateway.Str("collection_id", q.CollectionID))
		return trimChunks(chunks, topK), nil
	}
	return mergeScoreBreakdown(rerankerResultsToChunks(reranked, topK), chunks), nil
}

// mergeScoreBreakdown（G-12 命中调试）：reranker 结果只带 ID/Content/新分——
// 从融合前候选集并回分路分解与被丢弃的 MetadataJSON；RerankScore=重排后分数。
func mergeScoreBreakdown(reranked, originals []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	if len(originals) == 0 {
		return reranked
	}
	byID := make(map[string]biz.KnowledgeChunk, len(originals))
	for _, o := range originals {
		byID[o.ID] = o
	}
	for i := range reranked {
		reranked[i].RerankScore = reranked[i].Score
		o, ok := byID[reranked[i].ID]
		if !ok {
			continue
		}
		reranked[i].MetadataJSON = o.MetadataJSON
		reranked[i].DenseScore = o.DenseScore
		reranked[i].SparseScore = o.SparseScore
		reranked[i].RRFScore = o.RRFScore
		// G-33：图谱卡片候选的 EntityScore 同理必并回——卡片相似度分解要在
		// 重排后仍能回答「为什么命中」。
		reranked[i].EntityScore = o.EntityScore
		// G-06：reranker 结果只带 ID/Content/新分，ParentChunkID 必须从融合前
		// 候选集并回——否则重排后 Router 尾步回填父块语境的唯一触发键丢失（对抗评审 F2）。
		reranked[i].ParentChunkID = o.ParentChunkID
	}
	return reranked
}

func (r *Retriever) shouldRerank(q biz.KnowledgeSearchQuery) bool {
	if r == nil || r.reranker == nil {
		return false
	}
	if q.UseRerank != nil && !*q.UseRerank {
		return false
	}
	return true
}

func (r *Retriever) embedQuery(ctx context.Context, query string) ([]float32, error) {
	return embedSearchQuery(ctx, r.embedder, query)
}

func embedSearchQuery(ctx context.Context, embedder QueryEmbedder, query string) ([]float32, error) {
	if te, ok := embedder.(TaskTypeEmbedder); ok {
		return te.EmbedWithTaskType(ctx, query, "RETRIEVAL_QUERY")
	}
	return embedder.EmbedSingle(ctx, query)
}

func rerankCandidateLimit(q biz.KnowledgeSearchQuery, topK int) int {
	if q.RerankCandidates > 0 {
		if q.RerankCandidates > maxRerankOverfetch {
			return maxRerankOverfetch
		}
		return q.RerankCandidates
	}
	n := topK * 3
	if n < defaultRerankOverfetchMin {
		n = defaultRerankOverfetchMin
	}
	if n > maxRerankOverfetch {
		n = maxRerankOverfetch
	}
	return n
}

func trimChunks(chunks []biz.KnowledgeChunk, topK int) []biz.KnowledgeChunk {
	if topK <= 0 || len(chunks) <= topK {
		return chunks
	}
	return chunks[:topK]
}

func chunksToRerankerResults(chunks []biz.KnowledgeChunk) []*reranker.Result {
	out := make([]*reranker.Result, len(chunks))
	for i, ch := range chunks {
		meta := map[string]any{
			"chunk_id":      ch.ID,
			"doc_id":        ch.DocID,
			"collection_id": ch.CollectionID,
			"chunk_index":   ch.ChunkIndex,
		}
		out[i] = &reranker.Result{
			Document: &document.Document{
				ID:       ch.ID,
				Content:  ch.Content,
				Metadata: meta,
			},
			Score: float64(ch.Score),
		}
	}
	return out
}

func rerankerResultsToChunks(results []*reranker.Result, topK int) []biz.KnowledgeChunk {
	out := make([]biz.KnowledgeChunk, 0, len(results))
	for _, res := range results {
		if res == nil || res.Document == nil {
			continue
		}
		ch := biz.KnowledgeChunk{
			ID:      res.Document.ID,
			Content: res.Document.Content,
			Score:   float32(res.Score),
		}
		if res.Document.Metadata != nil {
			if v, ok := res.Document.Metadata["doc_id"].(string); ok {
				ch.DocID = v
			}
			if v, ok := res.Document.Metadata["collection_id"].(string); ok {
				ch.CollectionID = v
			}
			if v, ok := res.Document.Metadata["chunk_index"].(float64); ok {
				ch.ChunkIndex = int(v)
			}
		}
		out = append(out, ch)
	}
	return trimChunks(out, topK)
}
