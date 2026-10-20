package knowledge

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultRRF_K = 60

type HybridSearchMode string

const (
	HybridAuto   HybridSearchMode = "auto"
	HybridDense  HybridSearchMode = "dense"
	HybridSparse HybridSearchMode = "sparse"
	HybridRRF    HybridSearchMode = "rrf"
)

func ParseHybridSearchMode(raw string) HybridSearchMode {
	s := HybridSearchMode(raw)
	switch s {
	case HybridDense, HybridSparse, HybridRRF:
		return s
	default:
		return HybridAuto
	}
}

type SparseSearcher interface {
	SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
}

type HybridRetriever struct {
	embedder QueryEmbedder
	dense    biz.KnowledgeRepo
	sparse   SparseSearcher
	cards    GraphCardSearcher
	reranker rerankerForHybrid
	rrfK     int
	lg       loggateway.Logger
}

type rerankerForHybrid interface {
	HasReranker() bool
	RerankChunks(ctx context.Context, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk, topK int) ([]biz.KnowledgeChunk, error)
}

type retrieverAdapter struct {
	*Retriever
}

func (a *retrieverAdapter) HasReranker() bool {
	return a.Retriever != nil && a.Retriever.HasReranker()
}

func NewHybridRetriever(retriever *Retriever, sparse SparseSearcher, lg loggateway.Logger) *HybridRetriever {
	rrfK := defaultRRF_K
	return &HybridRetriever{
		embedder: retriever.embedder,
		dense:    retriever.repo,
		sparse:   sparse,
		reranker: &retrieverAdapter{retriever},
		rrfK:     rrfK,
		lg:       lg,
	}
}

// SetGraphCardSearcher 接线图谱卡片第三腿（G-33，ADR-KN-20261020-I）：
// 照 SparseSearcher 先例 nil=关闭——v1 只在 RRF 模式启用（dense-only 模式
// 与词法库降级路径不跑本腿，词法降级分支由同一检索器端口内部换实现备役）。
func (h *HybridRetriever) SetGraphCardSearcher(s GraphCardSearcher) {
	h.cards = s
}

func (h *HybridRetriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery, mode HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	totalStart := time.Now()
	defer observeSearchStage(searchStageTotal, totalStart)
	if h.dense == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "hybrid_retriever: dense repo is nil")
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 5
	}

	// F5：无语义层（embedder 未配置）时降级 BM25 词法检索——V2 设计 embedding 为
	// 可选增强，词法检索必须可用。
	if h.embedder == nil {
		h.logSparseFallback(q, "embedder is nil", nil)
		return h.searchSparse(ctx, q, topK)
	}

	// §V5 降级矩阵 #3：目标集合为无语义层词法库（embedding_model 空）时直接降级
	// BM25——chunks 无向量，dense/RRF 密集侧恒空。判定前置避免浪费一次查询 embed。
	if collectionLacksSemanticLayer(ctx, h.dense, q.CollectionID) {
		h.logSparseFallback(q, "collection has no semantic layer", nil)
		return h.searchSparse(ctx, q, topK)
	}

	effectiveMode := mode
	if effectiveMode == HybridAuto {
		effectiveMode = h.selectMode(q)
	}

	switch effectiveMode {
	case HybridSparse:
		return h.searchSparse(ctx, q, topK)
	case HybridRRF:
		return h.searchRRF(ctx, q, topK)
	case HybridDense:
		fallthrough
	default:
		embedStart := time.Now()
		vec, err := embedSearchQuery(ctx, h.embedder, q.Query)
		observeSearchStage(searchStageEmbed, embedStart)
		if err != nil {
			// F5：embedder 不可用（未配置/服务不可达）时降级 BM25 词法检索。
			if apierror.IsCode(err, apierror.CodeUnavailable) {
				h.logSparseFallback(q, "embed failed", err)
				return h.searchSparse(ctx, q, topK)
			}
			return nil, apierror.Internal(apierror.DomainKnowledge, "hybrid_retriever embed failed").WithCause(err)
		}
		searchQ := q
		searchQ.TopK = topK
		if h.reranker != nil && h.reranker.HasReranker() {
			searchQ.TopK = rerankCandidateLimit(q, topK)
		}
		denseStart := time.Now()
		chunks, err := h.dense.SearchChunks(ctx, searchQ, vec)
		observeSearchStage(searchStageDense, denseStart)
		if err != nil {
			return nil, err
		}
		tagDenseScores(chunks)
		if h.reranker != nil && h.reranker.HasReranker() && len(chunks) > 0 {
			return h.reranker.RerankChunks(ctx, q, chunks, topK)
		}
		return trimChunks(chunks, topK), nil
	}
}

func (h *HybridRetriever) selectMode(q biz.KnowledgeSearchQuery) HybridSearchMode {
	if h.sparse == nil {
		return HybridDense
	}
	return HybridRRF
}

// logSparseFallback 记录语义层 → BM25 词法检索降级（F5）。
func (h *HybridRetriever) logSparseFallback(q biz.KnowledgeSearchQuery, reason string, cause error) {
	fields := []loggateway.Field{
		loggateway.StepID("knowledge.hybrid.sparse_fallback"),
		loggateway.Str("collection_id", q.CollectionID),
		loggateway.Str("reason", reason),
	}
	if cause != nil {
		fields = append(fields, loggateway.Err(cause))
	}
	h.lg.Warn("语义层不可用，降级 BM25 词法检索", fields...)
}

func (h *HybridRetriever) searchSparse(ctx context.Context, q biz.KnowledgeSearchQuery, topK int) ([]biz.KnowledgeChunk, error) {
	if h.sparse == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "hybrid_retriever: sparse searcher not configured")
	}
	q.TopK = topK
	sparseStart := time.Now()
	chunks, err := h.sparse.SearchChunksBM25(ctx, q)
	observeSearchStage(searchStageSparse, sparseStart)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainKnowledge, "hybrid_retriever sparse failed").WithCause(err)
	}
	tagSparseScores(chunks)
	return trimChunks(chunks, topK), nil
}

func (h *HybridRetriever) searchRRF(ctx context.Context, q biz.KnowledgeSearchQuery, topK int) ([]biz.KnowledgeChunk, error) {
	embedStart := time.Now()
	vec, err := embedSearchQuery(ctx, h.embedder, q.Query)
	observeSearchStage(searchStageEmbed, embedStart)
	if err != nil {
		// F5：embedder 不可用时 RRF 降级为纯 BM25 词法检索。
		if apierror.IsCode(err, apierror.CodeUnavailable) {
			h.logSparseFallback(q, "rrf embed failed", err)
			return h.searchSparse(ctx, q, topK)
		}
		return nil, apierror.Internal(apierror.DomainKnowledge, "hybrid_retriever embed failed").WithCause(err)
	}

	overfetch := topK * 3
	if overfetch < 20 {
		overfetch = 20
	}
	if overfetch > 50 {
		overfetch = 50
	}
	if h.reranker != nil && h.reranker.HasReranker() {
		if rerankLimit := rerankCandidateLimit(q, topK); rerankLimit > overfetch {
			overfetch = rerankLimit
		}
	}

	denseQ := q
	denseQ.TopK = overfetch
	sparseQ := q
	sparseQ.TopK = overfetch

	var (
		denseChunks  []biz.KnowledgeChunk
		sparseChunks []biz.KnowledgeChunk
		cardChunks   []biz.KnowledgeChunk
		denseErr     error
		sparseErr    error
		cardErr      error
		wg           sync.WaitGroup
	)
	wg.Add(1)
	safego.Go(ctx, "knowledge.search.rrf_dense", func() {
		defer wg.Done()
		start := time.Now()
		denseChunks, denseErr = h.dense.SearchChunks(ctx, denseQ, vec)
		observeSearchStage(searchStageDense, start)
	})
	if h.sparse != nil {
		wg.Add(1)
		safego.Go(ctx, "knowledge.search.rrf_sparse", func() {
			defer wg.Done()
			start := time.Now()
			sparseChunks, sparseErr = h.sparse.SearchChunksBM25(ctx, sparseQ)
			observeSearchStage(searchStageSparse, start)
		})
	}
	// G-33 图谱卡片第三腿：与 dense/sparse 同模式并发；腿失败回退两腿融合
	// （warn 日志，照 dense/sparse 单路失败先例）。
	if h.cards != nil {
		wg.Add(1)
		safego.Go(ctx, "knowledge.search.rrf_graph_cards", func() {
			defer wg.Done()
			cardChunks, cardErr = h.cards.SearchGraphCardChunks(ctx, q, vec)
		})
	}
	wg.Wait()

	if denseErr != nil {
		h.lg.Warn("RRF 密集检索失败，回退稀疏",
			loggateway.StepID("knowledge.hybrid.dense_fail"),
			loggateway.Err(denseErr),
			loggateway.Str("collection_id", q.CollectionID))
		if h.sparse == nil || sparseErr != nil {
			return nil, apierror.Internal(apierror.DomainKnowledge, "hybrid_retriever dense and sparse failed").WithCause(denseErr)
		}
		return trimChunks(tagSparseScores(sparseChunks), topK), nil
	}

	if h.sparse == nil {
		return trimChunks(tagDenseScores(denseChunks), topK), nil
	}
	if sparseErr != nil {
		h.lg.Warn("RRF 稀疏检索失败，回退密集",
			loggateway.StepID("knowledge.hybrid.sparse_fail"),
			loggateway.Err(sparseErr),
			loggateway.Str("collection_id", q.CollectionID))
		return trimChunks(tagDenseScores(denseChunks), topK), nil
	}

	if cardErr != nil {
		h.lg.Warn("RRF 图谱卡片腿失败，回退密集+稀疏融合",
			loggateway.StepID("knowledge.hybrid.card_fail"),
			loggateway.Err(cardErr),
			loggateway.Str("collection_id", q.CollectionID))
		cardChunks = nil
	}

	merged := rrfMergeN(h.rrfK, denseChunks, sparseChunks, cardChunks)
	if h.reranker != nil && h.reranker.HasReranker() && len(merged) > 0 {
		return h.reranker.RerankChunks(ctx, q, merged, topK)
	}
	return trimChunks(merged, topK), nil
}

// tagDenseScores / tagSparseScores（G-12 命中调试）：单通路检索把该侧原始分
// 记入分路分解字段；最终 Score 语义不变。
func tagDenseScores(chunks []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	for i := range chunks {
		chunks[i].DenseScore = chunks[i].Score
	}
	return chunks
}

func tagSparseScores(chunks []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	for i := range chunks {
		chunks[i].SparseScore = chunks[i].Score
	}
	return chunks
}

func rrfMerge(dense, sparse []biz.KnowledgeChunk, k int) []biz.KnowledgeChunk {
	// G-33：两列包装（签名钉死：TestRRFMerge_ScoreBreakdown 等既有断言零改动），
	// 变参核心支持第三腿图谱卡片列。
	return rrfMergeN(k, dense, sparse)
}

// rrfMergeN 变参 RRF 融合核心（ADR-KN-20261020-I）：以加入序编号各列——
// 第 0/1 列的原始分记入 Dense/SparseScore 分解；第 ≥2 列（图谱卡片腿）原始
// 相似度携于 chunk.EntityScore，融合同名 ID 取最大。所有列贡献同权
// 1/(k+rank+1)（v1 不加权，金标裁决再调）。融合键恒为 ch.ID。
func rrfMergeN(k int, lists ...[]biz.KnowledgeChunk) []biz.KnowledgeChunk {
	scores := make(map[string]float64)
	denseRaw := make(map[string]float32)
	sparseRaw := make(map[string]float32)
	entityRaw := make(map[string]float32)
	chunkMap := make(map[string]*biz.KnowledgeChunk)

	// G-06 字面量重建纪律：融合重建处必须抄录全部经融合字段（G-06 钉——
	// 新增字段请同步此处，G-33 起含 EntityScore）。
	rebuild := func(ch biz.KnowledgeChunk) *biz.KnowledgeChunk {
		return &biz.KnowledgeChunk{
			ID:            ch.ID,
			DocID:         ch.DocID,
			CollectionID:  ch.CollectionID,
			Content:       ch.Content,
			MetadataJSON:  ch.MetadataJSON,
			ChunkIndex:    ch.ChunkIndex,
			ParentChunkID: ch.ParentChunkID,
			EntityScore:   ch.EntityScore,
		}
	}

	for li, list := range lists {
		for i, ch := range list {
			scores[ch.ID] += 1.0 / float64(k+i+1)
			switch li {
			case 0:
				denseRaw[ch.ID] = ch.Score
			case 1:
				sparseRaw[ch.ID] = ch.Score
			default:
				// 卡片腿：原始分语义在 EntityScore，不写入 dense/sparse 分解。
				if ch.EntityScore > entityRaw[ch.ID] {
					entityRaw[ch.ID] = ch.EntityScore
				}
			}
			if _, exists := chunkMap[ch.ID]; !exists {
				chunkMap[ch.ID] = rebuild(ch)
			}
		}
	}

	result := make([]biz.KnowledgeChunk, 0, len(chunkMap))
	for id, score := range scores {
		ch := chunkMap[id]
		fused := float32(score)
		// G-12 命中调试：Score==RRFScore 为融合排序键；两侧保留各自原始分。
		ch.Score = fused
		ch.RRFScore = fused
		ch.DenseScore = denseRaw[id]
		ch.SparseScore = sparseRaw[id]
		if e := entityRaw[id]; e > ch.EntityScore {
			ch.EntityScore = e
		}
		result = append(result, *ch)
	}

	sortChunksByRRFScoreDesc(result)
	return result
}

func sortChunksByRRFScoreDesc(chunks []biz.KnowledgeChunk) {
	for i := 1; i < len(chunks); i++ {
		for j := i; j > 0 && chunks[j].Score > chunks[j-1].Score; j-- {
			chunks[j], chunks[j-1] = chunks[j-1], chunks[j]
		}
	}
}

func MergeSearchResults(primary, supplement []biz.KnowledgeChunk, topK int) []biz.KnowledgeChunk {
	seen := make(map[string]struct{}, len(primary)+len(supplement))
	merged := make([]biz.KnowledgeChunk, 0, len(primary)+len(supplement))
	for _, ch := range primary {
		if _, ok := seen[ch.ID]; !ok {
			seen[ch.ID] = struct{}{}
			merged = append(merged, ch)
		}
	}
	for _, ch := range supplement {
		if _, ok := seen[ch.ID]; !ok {
			seen[ch.ID] = struct{}{}
			merged = append(merged, ch)
		}
	}
	sortChunksByScoreDesc(merged)
	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}
