// Package knowledge provides the knowledge_search tool for trpc Runners.
// The tool queries a named knowledge collection and returns relevant text chunks.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
		if scoped := knowledgeCollectionsFromContext(ctx); len(scoped) == 1 {
			in.CollectionID = scoped[0]
		} else if len(scoped) > 1 {
			return nil, fmt.Errorf("knowledge_search: collection_id is required when multiple knowledge_bases are scoped")
		} else {
			return nil, fmt.Errorf("knowledge_search: collection_id is required")
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
			return nil, fmt.Errorf("knowledge_search: collection_id %q is not in scoped knowledge_bases", in.CollectionID)
		}
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
