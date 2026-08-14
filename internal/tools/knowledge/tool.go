// Package knowledge provides the knowledge_search tool for trpc Runners.
// The tool queries a named knowledge collection and returns relevant text chunks.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"
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
// US-14：collection_id 可选——留空时按 scoped（Agent 绑定）/全库智能路由，用户无需选库。
type searchInput struct {
	CollectionID string  `json:"collection_id,omitempty" jsonschema:"description=The knowledge collection ID to search. Omit to smart-route across all accessible collections"`
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
		if in.Query == "" {
			return searchOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_search: query is required")
		}
		if in.TopK <= 0 {
			in.TopK = 5
		}

		scoped := knowledgeCollectionsFromContext(ctx)

		q := biz.KnowledgeSearchQuery{
			Query:      in.Query,
			TopK:       in.TopK,
			MinScore:   in.MinScore,
			FilterJSON: in.FilterJSON,
			UseRerank:  in.UseRerank,
		}

		var chunks []biz.KnowledgeChunk
		var err error

		// US-14 全库路由解析顺序：显式 ID → scoped==1 单库 → scoped>1 内路由 → 全库路由。
		switch {
		case in.CollectionID != "":
			if !collectionAllowed(scoped, in.CollectionID) {
				return searchOutput{}, apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("knowledge_search: collection_id %q is not in scoped knowledge_bases", in.CollectionID))
			}
			q.CollectionID = in.CollectionID
			chunks, err = searchSingleCollection(ctx, q)
		case len(scoped) == 1:
			q.CollectionID = scoped[0]
			chunks, err = searchSingleCollection(ctx, q)
		case len(scoped) > 1:
			fr := FederatedRetrieverFromContext(ctx)
			if fr == nil {
				return searchOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_search: federated retriever not configured for multi-collection search")
			}
			opts := knowledge.DefaultFederatedSearchOptions()
			opts.Strategy = knowledge.FederationRoute
			chunks, err = fr.SearchWithOptions(ctx, scoped, q, nil, "", opts)
		default:
			fr := FederatedRetrieverFromContext(ctx)
			if fr == nil {
				return searchOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_search: federated retriever not configured for collection-free search")
			}
			chunks, err = fr.SearchAll(ctx, q, nil, "")
		}
		if err != nil {
			return searchOutput{}, apierror.Internal(apierror.DomainKnowledge, fmt.Sprintf("knowledge_search: %s", err.Error()))
		}

		// 29-token P2-2: cited 回采 — 每次检索调用发射 knowledge_recalled
		// notice 携带返回 chunk 集合，供引用回填 worker 度量命中率闭环。
		emitKnowledgeRecalledNotice(ctx, chunks)

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
		function.WithDescription("Search knowledge bases for relevant text chunks using semantic similarity. Omit collection_id to smart-route across all accessible collections. Supports hybrid search (dense + sparse) and adaptive routing when available. Use this when you need factual information from knowledge bases. For multi-collection search or quality verification, use knowledge_reflect instead."),
	)
}

// collectionAllowed 报告显式 collection_id 是否在 scoped 白名单内（scoped 为空 = 不限定）。
func collectionAllowed(scoped []string, id string) bool {
	if len(scoped) == 0 {
		return true
	}
	for _, sid := range scoped {
		if sid == id {
			return true
		}
	}
	return false
}

// searchSingleCollection 单库直搜：优先 AdaptiveRouter，退化 Retriever。
func searchSingleCollection(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if router := AdaptiveRouterFromContext(ctx); router != nil {
		return router.Search(ctx, q, nil, "")
	}
	if ret := RetrieverFromContext(ctx); ret != nil {
		return ret.Search(ctx, q)
	}
	return nil, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_search: retriever not configured in context")
}

// ── knowledge_recalled transparency notice (29-token P2-2: cited 回采) ──────

const (
	// knowledgeRecalledMaxChunks caps the chunks carried by one notice payload.
	knowledgeRecalledMaxChunks = 10
	// knowledgeRecalledMaxLineRunes caps one chunk preview line inside the payload.
	knowledgeRecalledMaxLineRunes = 120
)

// knowledgeRecalledChunk is one returned chunk inside the notice payload.
// ChunkID is the citation-tracking key; Line is a short preview for potential
// UI rendering (the citation trace reader resolves full content by ChunkID).
type knowledgeRecalledChunk struct {
	ChunkID string  `json:"chunk_id"`
	DocID   string  `json:"doc_id,omitempty"`
	Score   float32 `json:"score,omitempty"`
	Line    string  `json:"line,omitempty"`
}

// knowledgeRecalledNoticePayload is the JSON content of a knowledge_recalled
// notice. Mirrored by the data-layer citation trace reader
// (internal/data/knowledge_citation.go).
type knowledgeRecalledNoticePayload struct {
	Chunks []knowledgeRecalledChunk `json:"chunks"`
}

// emitKnowledgeRecalledNotice emits one knowledge_recalled notice carrying the
// chunks returned by this retrieval call (best-effort, Informational per
// AS-EVT-01). No-op when there are no chunks or no ActivityEmitter in ctx
// (e.g. standalone tool execution outside a chat/team turn). Emit failures
// never break the tool call. Unlike the memory side there is no per-turn dedup
// guard: each retrieval call is a distinct candidate set, and one turn may
// legitimately carry multiple notices (the backfill reader handles that).
func emitKnowledgeRecalledNotice(ctx context.Context, chunks []biz.KnowledgeChunk) {
	if len(chunks) == 0 {
		return
	}
	emitter := biz.ActivityEmitterFromContext(ctx)
	if emitter == nil {
		return
	}
	payload := knowledgeRecalledNoticePayload{Chunks: make([]knowledgeRecalledChunk, 0, len(chunks))}
	for i, ch := range chunks {
		if i >= knowledgeRecalledMaxChunks {
			break
		}
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			continue
		}
		line := strings.TrimSpace(ch.Content)
		if r := []rune(line); len(r) > knowledgeRecalledMaxLineRunes {
			line = string(r[:knowledgeRecalledMaxLineRunes]) + "…"
		}
		payload.Chunks = append(payload.Chunks, knowledgeRecalledChunk{
			ChunkID: id,
			DocID:   strings.TrimSpace(ch.DocID),
			Score:   ch.Score,
			Line:    line,
		})
	}
	if len(payload.Chunks) == 0 {
		return
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return
	}
	// Emit failures are non-fatal: the notice is transparency/metrics-only and
	// must never break a tool call (same contract as the memory side).
	_ = emitter.EmitNotice(ctx, string(content), bizknowledge.KnowledgeRecalledNoticeType)
}

type reflectInput struct {
	CollectionIDs []string `json:"collection_ids,omitempty" jsonschema:"description=List of collection IDs to search across. Omit to smart-route across all accessible collections"`
	Query         string   `json:"query" jsonschema:"description=The original user query to reflect on,required"`
	TopK          int      `json:"top_k,omitempty" jsonschema:"description=Maximum number of results to return per collection"`
}

type reflectOutput struct {
	Sufficient      bool           `json:"sufficient"`
	Confidence      float32        `json:"confidence"`
	SupplementQuery string         `json:"supplement_query,omitempty"`
	Chunks          []chunkSummary `json:"chunks"`
}

func NewReflectTool(lg loggateway.Logger) trpctool.CallableTool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	execute := func(ctx context.Context, in reflectInput) (reflectOutput, error) {
		scoped := knowledgeCollectionsFromContext(ctx)
		// US-14：collection_ids 可选——留空时先用 scoped，仍为空则全库路由。
		if len(in.CollectionIDs) == 0 && len(scoped) > 0 {
			in.CollectionIDs = scoped
		}
		if len(scoped) > 0 {
			for _, id := range in.CollectionIDs {
				if !collectionAllowed(scoped, id) {
					return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("knowledge_reflect: collection_id %q is not in scoped knowledge_bases", id))
				}
			}
		}
		if in.Query == "" {
			return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_reflect: query is required")
		}
		if in.TopK <= 0 {
			in.TopK = 5
		}

		q := biz.KnowledgeSearchQuery{
			Query: in.Query,
			TopK:  in.TopK,
		}

		var chunks []biz.KnowledgeChunk
		var err error

		if fr := FederatedRetrieverFromContext(ctx); fr != nil {
			if len(in.CollectionIDs) == 0 {
				// US-14：无显式 ID 且无 scoped → 全库智能路由（零库返回空结果）。
				chunks, err = fr.SearchAll(ctx, q, nil, "")
			} else {
				chunks, err = fr.Search(ctx, in.CollectionIDs, q, nil, "")
			}
		} else if len(in.CollectionIDs) == 0 {
			return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_reflect: collection_ids is required")
		} else if router := AdaptiveRouterFromContext(ctx); router != nil {
			if len(in.CollectionIDs) == 1 {
				q.CollectionID = in.CollectionIDs[0]
				chunks, err = router.Search(ctx, q, nil, "")
			} else {
				return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_reflect: federated retriever not configured for multi-collection search")
			}
		} else if ret := RetrieverFromContext(ctx); ret != nil {
			if len(in.CollectionIDs) == 1 {
				q.CollectionID = in.CollectionIDs[0]
				chunks, err = ret.Search(ctx, q)
			} else {
				return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_reflect: federated retriever not configured for multi-collection search")
			}
		} else {
			return reflectOutput{}, apierror.BadRequest(apierror.DomainKnowledge, "knowledge_reflect: retriever not configured in context")
		}
		if err != nil {
			return reflectOutput{}, apierror.Internal(apierror.DomainKnowledge, fmt.Sprintf("knowledge_reflect: %s", err.Error()))
		}

		// 29-token P2-2: 同 knowledge_search — 反射检索同样发射回采 notice。
		emitKnowledgeRecalledNotice(ctx, chunks)

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
		function.WithDescription("Reflect on knowledge search results across collections. Omit collection_ids to smart-route across all accessible collections. Evaluates retrieval quality, determines if results are sufficient, and suggests supplementary queries if needed. Use this after knowledge_search when you need to verify result quality or search across multiple knowledge bases."),
	)
}
