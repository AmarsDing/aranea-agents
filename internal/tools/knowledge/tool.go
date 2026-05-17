// Package knowledge provides the knowledge_search tool for trpc Runners.
// The tool queries a named knowledge collection and returns relevant text chunks.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// contextKey is the key used to store the retriever in context.
type contextKey struct{}

// WithRetriever attaches a Retriever to the context for use by SearchTool.
func WithRetriever(ctx context.Context, r *knowledge.Retriever) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}

// RetrieverFromContext extracts the Retriever from ctx.
func RetrieverFromContext(ctx context.Context) *knowledge.Retriever {
	r, _ := ctx.Value(contextKey{}).(*knowledge.Retriever)
	return r
}

// searchInput is the JSON schema for the knowledge_search tool.
type searchInput struct {
	CollectionID string  `json:"collection_id"`
	Query        string  `json:"query"`
	TopK         int     `json:"top_k,omitempty"`
	MinScore     float32 `json:"min_score,omitempty"`
}

// searchOutput is the structured result returned to the model.
type searchOutput struct {
	Chunks []chunkSummary `json:"chunks"`
}

type chunkSummary struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Score   float32 `json:"score"`
	DocID   string  `json:"doc_id"`
}

// NewSearchTool returns the knowledge_search tool.
func NewSearchTool() trpctool.CallableTool {
	return &searchTool{}
}

type searchTool struct{}

func (t *searchTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "knowledge_search",
		Description: "Search a knowledge collection for relevant text chunks using semantic similarity.",
	}
}

func (t *searchTool) Call(ctx context.Context, args []byte) (any, error) {
	var in searchInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("knowledge_search: invalid args: %w", err)
	}
	if in.CollectionID == "" {
		return nil, fmt.Errorf("knowledge_search: collection_id is required")
	}
	if in.Query == "" {
		return nil, fmt.Errorf("knowledge_search: query is required")
	}
	if in.TopK <= 0 {
		in.TopK = 5
	}

	ret := RetrieverFromContext(ctx)
	if ret == nil {
		return nil, fmt.Errorf("knowledge_search: retriever not configured in context")
	}

	chunks, err := ret.Search(ctx, biz.KnowledgeSearchQuery{
		CollectionID: in.CollectionID,
		Query:        in.Query,
		TopK:         in.TopK,
		MinScore:     in.MinScore,
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge_search: %w", err)
	}

	out := searchOutput{Chunks: make([]chunkSummary, 0, len(chunks))}
	for _, ch := range chunks {
		out.Chunks = append(out.Chunks, chunkSummary{
			ID:      ch.ID,
			Content: ch.Content,
			Score:   ch.Score,
			DocID:   ch.DocID,
		})
	}
	return out, nil
}
