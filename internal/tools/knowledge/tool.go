// Package knowledge provides the knowledge_search tool for trpc Runners.
// The tool queries a named knowledge collection and returns relevant text chunks.
package knowledge

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// contextKey is the key used to store the retriever in context.
type contextKey struct{}

// WithRetriever attaches a Retriever to the context for use by SearchTool.
func WithRetriever(ctx context.Context, r *knowledge.Retriever) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}

type collectionsKey struct{}

// WithKnowledgeCollections restricts knowledge_search to the given collection IDs for this turn.
func WithKnowledgeCollections(ctx context.Context, collectionIDs []string) context.Context {
	filtered := make([]string, 0, len(collectionIDs))
	for _, id := range collectionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return ctx
	}
	return context.WithValue(ctx, collectionsKey{}, filtered)
}

func knowledgeCollectionsFromContext(ctx context.Context) []string {
	raw, _ := ctx.Value(collectionsKey{}).([]string)
	return raw
}

// RetrieverFromContext extracts the Retriever from ctx.
func RetrieverFromContext(ctx context.Context) *knowledge.Retriever {
	r, _ := ctx.Value(contextKey{}).(*knowledge.Retriever)
	return r
}

// searchInput is the JSON schema for the knowledge_search tool.
type searchInput struct {
	CollectionID string  `json:"collection_id" jsonschema:"description=The knowledge collection ID to search,required"`
	Query        string  `json:"query" jsonschema:"description=Natural-language search query,required"`
	TopK         int     `json:"top_k,omitempty" jsonschema:"description=Maximum number of results to return"`
	MinScore     float32 `json:"min_score,omitempty" jsonschema:"description=Minimum similarity score threshold"`
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
	execute := func(ctx context.Context, in searchInput) (searchOutput, error) {
		if in.CollectionID == "" {
			if scoped := knowledgeCollectionsFromContext(ctx); len(scoped) == 1 {
				in.CollectionID = scoped[0]
			} else if len(scoped) > 1 {
				return searchOutput{}, fmt.Errorf("knowledge_search: collection_id is required when multiple knowledge_bases are scoped")
			} else {
				return searchOutput{}, fmt.Errorf("knowledge_search: collection_id is required")
			}
		}
		if scoped := knowledgeCollectionsFromContext(ctx); len(scoped) > 0 {
			allowed := false
			for _, id := range scoped {
				if id == in.CollectionID {
					allowed = true
					break
				}
			}
			if !allowed {
				return searchOutput{}, fmt.Errorf("knowledge_search: collection_id %q is not in scoped knowledge_bases", in.CollectionID)
			}
		}
		if in.Query == "" {
			return searchOutput{}, fmt.Errorf("knowledge_search: query is required")
		}
		if in.TopK <= 0 {
			in.TopK = 5
		}

		ret := RetrieverFromContext(ctx)
		if ret == nil {
			return searchOutput{}, fmt.Errorf("knowledge_search: retriever not configured in context")
		}

		chunks, err := ret.Search(ctx, biz.KnowledgeSearchQuery{
			CollectionID: in.CollectionID,
			Query:        in.Query,
			TopK:         in.TopK,
			MinScore:     in.MinScore,
		})
		if err != nil {
			return searchOutput{}, fmt.Errorf("knowledge_search: %w", err)
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
	return function.NewFunctionTool(
		execute,
		function.WithName("knowledge_search"),
		function.WithDescription("Search a knowledge collection for relevant text chunks using semantic similarity."),
	)
}
