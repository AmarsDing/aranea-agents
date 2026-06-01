// Package knowledge provides vector retrieval for knowledge base search.
package knowledge

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
)

const defaultRerankOverfetchMin = 20
const maxRerankOverfetch = 50

// QueryEmbedder embeds search queries.
type QueryEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
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
}

// NewRetriever creates a Retriever bound to the given embedder, repo, and optional reranker.
func NewRetriever(embedder QueryEmbedder, repo biz.KnowledgeRepo, rr reranker.Reranker) *Retriever {
	return &Retriever{embedder: embedder, repo: repo, reranker: rr}
}

// HasReranker reports whether a reranker is configured globally.
func (r *Retriever) HasReranker() bool {
	return r != nil && r.reranker != nil
}

// Search embeds the query, retrieves candidates, optionally reranks, and returns top-k chunks.
func (r *Retriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if r.embedder == nil {
		return nil, kerrors.ServiceUnavailable("KNOWLEDGE", "retriever: embedder is nil")
	}
	if r.repo == nil {
		return nil, kerrors.ServiceUnavailable("KNOWLEDGE", "retriever: repo is nil")
	}
	topK := q.TopK
	if topK <= 0 {
		topK = 5
	}

	vec, err := r.embedQuery(ctx, q.Query)
	if err != nil {
		return nil, kerrors.InternalServer("KNOWLEDGE", "retriever embed failed: "+err.Error())
	}

	searchQ := q
	searchQ.TopK = topK
	if r.shouldRerank(q) {
		searchQ.TopK = rerankCandidateLimit(q, topK)
	}

	chunks, err := r.repo.SearchChunks(ctx, searchQ, vec)
	if err != nil {
		return nil, err
	}
	if !r.shouldRerank(q) || len(chunks) == 0 {
		return trimChunks(chunks, topK), nil
	}

	results := chunksToRerankerResults(chunks)
	reranked, err := r.reranker.Rerank(ctx, &reranker.Query{Text: q.Query, FinalQuery: q.Query}, results)
	if err != nil {
		loggateway.Global().Warn("重排失败，使用向量排序",
			loggateway.StepID("knowledge.rerank.fallback"),
			loggateway.Err(err),
			loggateway.Str("collection_id", q.CollectionID))
		return trimChunks(chunks, topK), nil
	}
	return rerankerResultsToChunks(reranked, topK), nil
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
	if te, ok := r.embedder.(TaskTypeEmbedder); ok {
		return te.EmbedWithTaskType(ctx, query, "RETRIEVAL_QUERY")
	}
	return r.embedder.Embed(ctx, query)
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
