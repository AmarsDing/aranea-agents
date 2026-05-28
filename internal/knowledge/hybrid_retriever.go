package knowledge

import (
	"context"
	"fmt"
	"math"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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
	reranker rerankerForHybrid
	rrfK     int
}

type rerankerForHybrid interface {
	HasReranker() bool
	searchWithRerank(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error)
}

type retrieverAdapter struct {
	*Retriever
}

func (a *retrieverAdapter) HasReranker() bool {
	return a.Retriever != nil && a.Retriever.HasReranker()
}

func (a *retrieverAdapter) searchWithRerank(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	return a.Retriever.Search(ctx, q)
}

func NewHybridRetriever(retriever *Retriever, sparse SparseSearcher) *HybridRetriever {
	rrfK := defaultRRF_K
	return &HybridRetriever{
		embedder: retriever.embedder,
		dense:    retriever.repo,
		sparse:   sparse,
		reranker: &retrieverAdapter{retriever},
		rrfK:     rrfK,
	}
}

func (h *HybridRetriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery, mode HybridSearchMode) ([]biz.KnowledgeChunk, error) {
	if h.embedder == nil {
		return nil, fmt.Errorf("hybrid_retriever: embedder is nil")
	}
	if h.dense == nil {
		return nil, fmt.Errorf("hybrid_retriever: dense repo is nil")
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 5
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
		vec, err := h.embedder.Embed(ctx, q.Query)
		if err != nil {
			return nil, fmt.Errorf("hybrid_retriever embed: %w", err)
		}
		searchQ := q
		searchQ.TopK = topK
		if h.reranker != nil && h.reranker.HasReranker() {
			searchQ.TopK = rerankCandidateLimit(q, topK)
		}
		chunks, err := h.dense.SearchChunks(ctx, searchQ, vec)
		if err != nil {
			return nil, err
		}
		if h.reranker != nil && h.reranker.HasReranker() && len(chunks) > 0 {
			return h.reranker.searchWithRerank(ctx, q, vec)
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

func (h *HybridRetriever) searchSparse(ctx context.Context, q biz.KnowledgeSearchQuery, topK int) ([]biz.KnowledgeChunk, error) {
	if h.sparse == nil {
		return nil, fmt.Errorf("hybrid_retriever: sparse searcher not configured")
	}
	q.TopK = topK
	chunks, err := h.sparse.SearchChunksBM25(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("hybrid_retriever sparse: %w", err)
	}
	return trimChunks(chunks, topK), nil
}

func (h *HybridRetriever) searchRRF(ctx context.Context, q biz.KnowledgeSearchQuery, topK int) ([]biz.KnowledgeChunk, error) {
	vec, err := h.embedder.Embed(ctx, q.Query)
	if err != nil {
		return nil, fmt.Errorf("hybrid_retriever embed: %w", err)
	}

	overfetch := topK * 3
	if overfetch < 20 {
		overfetch = 20
	}
	if overfetch > 50 {
		overfetch = 50
	}

	denseQ := q
	denseQ.TopK = overfetch
	denseChunks, err := h.dense.SearchChunks(ctx, denseQ, vec)
	if err != nil {
		event.SysLogWarn("knowledge.hybrid.dense_fail", "RRF 密集检索失败，回退稀疏",
			event.P("error", err.Error()), event.P("collection_id", q.CollectionID))
		return h.searchSparse(ctx, q, topK)
	}

	if h.sparse == nil {
		return trimChunks(denseChunks, topK), nil
	}

	sparseQ := q
	sparseQ.TopK = overfetch
	sparseChunks, err := h.sparse.SearchChunksBM25(ctx, sparseQ)
	if err != nil {
		event.SysLogWarn("knowledge.hybrid.sparse_fail", "RRF 稀疏检索失败，回退密集",
			event.P("error", err.Error()), event.P("collection_id", q.CollectionID))
		return trimChunks(denseChunks, topK), nil
	}

	merged := rrfMerge(denseChunks, sparseChunks, h.rrfK)
	return trimChunks(merged, topK), nil
}

func rrfMerge(dense, sparse []biz.KnowledgeChunk, k int) []biz.KnowledgeChunk {
	scores := make(map[string]float64)
	chunkMap := make(map[string]*biz.KnowledgeChunk)

	for i, ch := range dense {
		scores[ch.ID] += 1.0 / float64(k+i+1)
		if _, exists := chunkMap[ch.ID]; !exists {
			chunkMap[ch.ID] = &biz.KnowledgeChunk{
				ID:           ch.ID,
				DocID:        ch.DocID,
				CollectionID: ch.CollectionID,
				Content:      ch.Content,
				MetadataJSON: ch.MetadataJSON,
				ChunkIndex:   ch.ChunkIndex,
			}
		}
	}

	for i, ch := range sparse {
		scores[ch.ID] += 1.0 / float64(k+i+1)
		if _, exists := chunkMap[ch.ID]; !exists {
			chunkMap[ch.ID] = &biz.KnowledgeChunk{
				ID:           ch.ID,
				DocID:        ch.DocID,
				CollectionID: ch.CollectionID,
				Content:      ch.Content,
				MetadataJSON: ch.MetadataJSON,
				ChunkIndex:   ch.ChunkIndex,
			}
		}
	}

	result := make([]biz.KnowledgeChunk, 0, len(chunkMap))
	for id, score := range scores {
		ch := chunkMap[id]
		ch.Score = float32(score)
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

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
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
