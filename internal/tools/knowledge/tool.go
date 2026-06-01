// Package knowledge provides the knowledge_search tool for trpc Runners.
// The tool queries a named knowledge collection and returns relevant text chunks.
package knowledge

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type contextKey struct{}

type routerKey struct{}

type federatedKey struct{}

type evaluatorKey struct{}

func WithRetriever(ctx context.Context, r *knowledge.Retriever) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}

func WithAdaptiveRouter(ctx context.Context, router *knowledge.AdaptiveRouter) context.Context {
	return context.WithValue(ctx, routerKey{}, router)
}

func WithFederatedRetriever(ctx context.Context, fr *knowledge.FederatedRetriever) context.Context {
	return context.WithValue(ctx, federatedKey{}, fr)
}

func WithRetrievalEvaluator(ctx context.Context, ev *knowledge.RetrievalEvaluator) context.Context {
	return context.WithValue(ctx, evaluatorKey{}, ev)
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

func KnowledgeCollectionsFromContext(ctx context.Context) []string {
	return knowledgeCollectionsFromContext(ctx)
}

func RetrieverFromContext(ctx context.Context) *knowledge.Retriever {
	r, _ := ctx.Value(contextKey{}).(*knowledge.Retriever)
	return r
}

func AdaptiveRouterFromContext(ctx context.Context) *knowledge.AdaptiveRouter {
	r, _ := ctx.Value(routerKey{}).(*knowledge.AdaptiveRouter)
	return r
}

func FederatedRetrieverFromContext(ctx context.Context) *knowledge.FederatedRetriever {
	fr, _ := ctx.Value(federatedKey{}).(*knowledge.FederatedRetriever)
	return fr
}

func RetrievalEvaluatorFromContext(ctx context.Context) *knowledge.RetrievalEvaluator {
	ev, _ := ctx.Value(evaluatorKey{}).(*knowledge.RetrievalEvaluator)
	return ev
}

// searchInput is the JSON schema for the knowledge_search tool.
type searchInput struct {
	CollectionID string  `json:"collection_id" jsonschema:"description=The knowledge collection ID to search,required"`
	Query        string  `json:"query" jsonschema:"description=Natural-language search query,required"`
	TopK         int     `json:"top_k,omitempty" jsonschema:"description=Maximum number of results to return"`
	MinScore     float32 `json:"min_score,omitempty" jsonschema:"description=Minimum similarity score threshold"`
	FilterJSON   string  `json:"filter_json,omitempty" jsonschema:"description=JSON metadata filter, e.g. {\"category\":\"tech\"}"`
	UseRerank    *bool   `json:"use_rerank,omitempty" jsonschema:"description=Whether to rerank results for improved relevance"`
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
				return searchOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_search: collection_id is required when multiple knowledge_bases are scoped")
			} else {
				return searchOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_search: collection_id is required")
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
				return searchOutput{}, kerrors.BadRequest("KNOWLEDGE", fmt.Sprintf("knowledge_search: collection_id %q is not in scoped knowledge_bases", in.CollectionID))
			}
		}
		if in.Query == "" {
			return searchOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_search: query is required")
		}
		if in.TopK <= 0 {
			in.TopK = 5
		}

		q := biz.KnowledgeSearchQuery{
			CollectionID: in.CollectionID,
			Query:        in.Query,
			TopK:         in.TopK,
			MinScore:     in.MinScore,
			FilterJSON:   in.FilterJSON,
			UseRerank:    in.UseRerank,
		}

		var chunks []biz.KnowledgeChunk
		var err error

		if router := AdaptiveRouterFromContext(ctx); router != nil {
			chunks, err = router.Search(ctx, q, nil, "")
		} else if ret := RetrieverFromContext(ctx); ret != nil {
			chunks, err = ret.Search(ctx, q)
		} else {
			return searchOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_search: retriever not configured in context")
		}
		if err != nil {
			return searchOutput{}, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("knowledge_search: %s", err.Error()))
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
		function.WithDescription("Search a knowledge collection for relevant text chunks using semantic similarity. Supports hybrid search (dense + sparse) and adaptive routing when available."),
	)
}

type reflectInput struct {
	CollectionIDs []string `json:"collection_ids" jsonschema:"description=List of collection IDs to search across,required"`
	Query         string   `json:"query" jsonschema:"description=The original user query to reflect on,required"`
	TopK          int      `json:"top_k,omitempty" jsonschema:"description=Maximum number of results to return per collection"`
}

type reflectOutput struct {
	Sufficient       bool           `json:"sufficient"`
	Confidence       float32        `json:"confidence"`
	SupplementQuery  string         `json:"supplement_query,omitempty"`
	Chunks           []chunkSummary `json:"chunks"`
}

func NewReflectTool(lg loggateway.Logger) trpctool.CallableTool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	execute := func(ctx context.Context, in reflectInput) (reflectOutput, error) {
		if len(in.CollectionIDs) == 0 {
			if scoped := knowledgeCollectionsFromContext(ctx); len(scoped) > 0 {
				in.CollectionIDs = scoped
			} else {
				return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_reflect: collection_ids is required")
			}
		}
		if scoped := knowledgeCollectionsFromContext(ctx); len(scoped) > 0 {
			for _, id := range in.CollectionIDs {
				allowed := false
				for _, sid := range scoped {
					if id == sid {
						allowed = true
						break
					}
				}
				if !allowed {
					return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", fmt.Sprintf("knowledge_reflect: collection_id %q is not in scoped knowledge_bases", id))
				}
			}
		}
		if in.Query == "" {
			return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_reflect: query is required")
		}
		if in.TopK <= 0 {
			in.TopK = 5
		}

		q := biz.KnowledgeSearchQuery{
			Query:    in.Query,
			TopK:     in.TopK,
		}

		var chunks []biz.KnowledgeChunk
		var err error

		if fr := FederatedRetrieverFromContext(ctx); fr != nil {
			chunks, err = fr.Search(ctx, in.CollectionIDs, q, nil, "")
		} else if router := AdaptiveRouterFromContext(ctx); router != nil {
			if len(in.CollectionIDs) == 1 {
				q.CollectionID = in.CollectionIDs[0]
				chunks, err = router.Search(ctx, q, nil, "")
			} else {
				return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_reflect: federated retriever not configured for multi-collection search")
			}
		} else if ret := RetrieverFromContext(ctx); ret != nil {
			if len(in.CollectionIDs) == 1 {
				q.CollectionID = in.CollectionIDs[0]
				chunks, err = ret.Search(ctx, q)
			} else {
				return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_reflect: federated retriever not configured for multi-collection search")
			}
		} else {
			return reflectOutput{}, kerrors.BadRequest("KNOWLEDGE", "knowledge_reflect: retriever not configured in context")
		}
		if err != nil {
			return reflectOutput{}, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("knowledge_reflect: %s", err.Error()))
		}

		out := reflectOutput{
			Sufficient: true,
			Confidence: 1.0,
			Chunks:     make([]chunkSummary, 0, len(chunks)),
		}

		if ev := RetrievalEvaluatorFromContext(ctx); ev != nil && len(chunks) > 0 {
			assessment, evalErr := ev.Evaluate(ctx, in.Query, chunks)
			if evalErr == nil {
				out.Sufficient = assessment.Sufficient
				out.Confidence = assessment.Confidence
				out.SupplementQuery = assessment.SupplementQuery
			} else {
				lg.Warn("evaluation failed",
					loggateway.StepID("tool.knowledge_reflect.eval_fail"),
					loggateway.Err(evalErr))
			}
		}

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
		function.WithName("knowledge_reflect"),
		function.WithDescription("Reflect on knowledge search results across multiple collections. Evaluates retrieval quality, determines if results are sufficient, and suggests supplementary queries if needed. Use this after knowledge_search when you need to verify result quality or search across multiple knowledge bases."),
	)
}
