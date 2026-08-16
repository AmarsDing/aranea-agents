package knowledge

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/knowledge"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
)

// SearchFunc is the signature shared by all project retrievers.
// It accepts a biz.KnowledgeSearchQuery and returns ranked chunks.
type SearchFunc func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)

// KnowledgeAdapter bridges the framework's knowledge.Knowledge interface
// to the project's self-built retrieval system (Retriever / HybridRetriever /
// AdaptiveRouter / FederatedRetriever).
type KnowledgeAdapter struct {
	searchFunc SearchFunc
	lg         loggateway.Logger
}

// Compile-time interface assertion.
var _ knowledge.Knowledge = (*KnowledgeAdapter)(nil)

// NewKnowledgeAdapter creates a KnowledgeAdapter with the given search function and logger.
func NewKnowledgeAdapter(fn SearchFunc, lg loggateway.Logger) *KnowledgeAdapter {
	return &KnowledgeAdapter{
		searchFunc: fn,
		lg:         lg.With(loggateway.Domain("knowledge_adapter")),
	}
}

// Search implements knowledge.Knowledge.
// It converts the framework SearchRequest to a biz.KnowledgeSearchQuery,
// delegates to the injected search function, and converts the results back.
func (a *KnowledgeAdapter) Search(ctx context.Context, req *knowledge.SearchRequest) (*knowledge.SearchResult, error) {
	if req == nil {
		return &knowledge.SearchResult{}, nil
	}

	bizQ := a.toBizQuery(req)

	chunks, err := a.searchFunc(ctx, bizQ)
	if err != nil {
		a.lg.Warn("知识检索失败",
			loggateway.StepID("knowledge_adapter.search_fail"),
			loggateway.Err(err),
			loggateway.Str("query", req.Query))
		return nil, err
	}

	return a.toSearchResult(chunks), nil
}

// toBizQuery converts a framework SearchRequest to a biz KnowledgeSearchQuery.
func (a *KnowledgeAdapter) toBizQuery(req *knowledge.SearchRequest) biz.KnowledgeSearchQuery {
	q := biz.KnowledgeSearchQuery{
		Query:    req.Query,
		TopK:     req.MaxResults,
		MinScore: float32(req.MinScore),
		// 词条优先写回：日记流水（inbox/writeback-*）仅 provenance，不进 Agent 默认
		// 检索——与 knowledge 工具/cue 预检索同规（此前框架原生 knowledge_search
		// 路径漏排，流水会进 Agent 上下文）。
		ExcludePathPrefixes: []string{biz.KnowledgeWriteBackInboxPrefix},
	}

	if q.TopK <= 0 {
		q.TopK = 5
	}

	// Extract collection_id from SearchFilter metadata if present.
	if req.SearchFilter != nil {
		if req.SearchFilter.Metadata != nil {
			if cid, ok := req.SearchFilter.Metadata["collection_id"].(string); ok {
				q.CollectionID = cid
			}
		}
		// Serialize filter metadata as FilterJSON for downstream consumption.
		// Exclude collection_id (already extracted above) and skip FilterCondition
		// (downstream SQL uses JSONB containment which cannot express complex conditions).
		if req.SearchFilter.Metadata != nil {
			filterMap := make(map[string]any)
			for k, v := range req.SearchFilter.Metadata {
				if k == "collection_id" {
					continue
				}
				filterMap[k] = v
			}
			if len(filterMap) > 0 {
				if raw, err := json.Marshal(filterMap); err == nil {
					q.FilterJSON = string(raw)
				}
			}
		}
	}

	return q
}

// toSearchResult converts biz chunks to a framework SearchResult.
func (a *KnowledgeAdapter) toSearchResult(chunks []biz.KnowledgeChunk) *knowledge.SearchResult {
	if len(chunks) == 0 {
		return &knowledge.SearchResult{}
	}

	docs := make([]*knowledge.Result, len(chunks))
	var bestDoc *document.Document
	var bestScore float64
	var bestText string

	for i, ch := range chunks {
		meta := map[string]any{
			"doc_id":        ch.DocID,
			"collection_id": ch.CollectionID,
			"chunk_index":   ch.ChunkIndex,
		}
		// Preserve original chunk metadata if present.
		if ch.MetadataJSON != "" {
			var origMeta map[string]any
			if json.Unmarshal([]byte(ch.MetadataJSON), &origMeta) == nil {
				for k, v := range origMeta {
					meta[k] = v
				}
			}
		}

		doc := &document.Document{
			ID:       ch.ID,
			Content:  ch.Content,
			Metadata: meta,
		}
		score := float64(ch.Score)
		docs[i] = &knowledge.Result{
			Document: doc,
			Score:    score,
		}

		if score > bestScore {
			bestScore = score
			bestDoc = doc
			bestText = ch.Content
		}
	}

	return &knowledge.SearchResult{
		Document:  bestDoc,
		Score:     bestScore,
		Text:      bestText,
		Documents: docs,
	}
}
