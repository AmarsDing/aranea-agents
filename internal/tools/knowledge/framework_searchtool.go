// Package knowledge provides knowledge search tools for trpc Runners.
//
// TECH-DEBT(B-4): framework-aligned search tools 已实现但未接入生产路径。
// 阻塞原因：(1) 双重注册冲突——项目已在 tool.go 注册自建 knowledge_search 工具，
// 此处使用框架 knowledge.NewKnowledgeSearchTool 会创建同名工具导致冲突；
// (2) 能力降级——项目自建 SearchTool 支持通过 knowledgetool.WithKnowledgeCollections
// 在 per-run context 中动态限定 collection，框架版本不支持此能力。
// 解除条件：框架 KnowledgeSearchTool 支持动态 collection 限定，或项目决定放弃
// 动态 collection 过滤能力。详见 alignment-plan.md §十一/B-4。
package knowledge

import (
	knowledgetool "trpc.group/trpc-go/trpc-agent-go/knowledge/tool"

	"trpc.group/trpc-go/trpc-agent-go/knowledge"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/searchfilter"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// FrameworkSearchToolOption configures a framework-aligned search tool.
type FrameworkSearchToolOption func(*frameworkSearchToolOptions)

type frameworkSearchToolOptions struct {
	collectionIDs []string
	toolName      string
	maxResults    int
	minScore      float64
}

// WithFrameworkCollections restricts the search tool to the given collection IDs.
// When provided, a static filter is applied so that only documents belonging to
// the specified collections are returned.
func WithFrameworkCollections(collectionIDs ...string) FrameworkSearchToolOption {
	return func(opts *frameworkSearchToolOptions) {
		opts.collectionIDs = collectionIDs
	}
}

// WithFrameworkToolName overrides the default tool name.
func WithFrameworkToolName(name string) FrameworkSearchToolOption {
	return func(opts *frameworkSearchToolOptions) {
		opts.toolName = name
	}
}

// WithFrameworkMaxResults sets the maximum number of results.
func WithFrameworkMaxResults(n int) FrameworkSearchToolOption {
	return func(opts *frameworkSearchToolOptions) {
		opts.maxResults = n
	}
}

// WithFrameworkMinScore sets the minimum relevance score threshold.
func WithFrameworkMinScore(score float64) FrameworkSearchToolOption {
	return func(opts *frameworkSearchToolOptions) {
		opts.minScore = score
	}
}

// NewFrameworkSearchTool creates a knowledge search tool backed by the
// framework's knowledge.NewKnowledgeSearchTool.
//
// Unlike the self-built NewSearchTool (which returns a CallableTool and relies
// on context-injected Retriever/AdaptiveRouter), this version takes a
// knowledge.Knowledge adapter directly and delegates all search logic to the
// framework — including conversation history extraction, filter merging, and
// result formatting.
//
// Collection scoping is supported via WithFrameworkCollections, which maps to
// the framework's WithFilter (single collection) or WithConditionedFilter
// (multiple collections) option as a static constraint.
func NewFrameworkSearchTool(kb knowledge.Knowledge, opts ...FrameworkSearchToolOption) tool.Tool {
	opt := &frameworkSearchToolOptions{}
	for _, o := range opts {
		o(opt)
	}

	var frameworkOpts []knowledgetool.Option

	// Map collection scoping to framework filter options.
	if len(opt.collectionIDs) == 1 {
		// Single collection: simple metadata eq filter.
		frameworkOpts = append(frameworkOpts, knowledgetool.WithFilter(
			map[string]any{"collection_id": opt.collectionIDs[0]},
		))
	} else if len(opt.collectionIDs) > 1 {
		// Multiple collections: use "in" operator via conditioned filter.
		frameworkOpts = append(frameworkOpts, knowledgetool.WithConditionedFilter(
			&searchfilter.UniversalFilterCondition{
				Field:    "collection_id",
				Operator: searchfilter.OperatorIn,
				Value:    opt.collectionIDs,
			},
		))
	}

	if opt.toolName != "" {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithToolName(opt.toolName))
	}
	if opt.maxResults > 0 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithMaxResults(opt.maxResults))
	}
	if opt.minScore > 0 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithMinScore(opt.minScore))
	}

	return knowledgetool.NewKnowledgeSearchTool(kb, frameworkOpts...)
}

// NewFrameworkAgenticFilterSearchTool creates a knowledge search tool with
// dynamic agent-controlled filtering, backed by the framework's
// knowledge.NewAgenticFilterSearchTool.
//
// The agentic filter allows the LLM to construct filter conditions dynamically
// based on available metadata fields and values, enabling more precise
// multi-turn retrieval.
func NewFrameworkAgenticFilterSearchTool(
	kb knowledge.Knowledge,
	agenticFilterInfo map[string][]any,
	opts ...FrameworkSearchToolOption,
) tool.Tool {
	opt := &frameworkSearchToolOptions{}
	for _, o := range opts {
		o(opt)
	}

	var frameworkOpts []knowledgetool.Option

	// Map collection scoping to framework filter options.
	if len(opt.collectionIDs) == 1 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithFilter(
			map[string]any{"collection_id": opt.collectionIDs[0]},
		))
	} else if len(opt.collectionIDs) > 1 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithConditionedFilter(
			&searchfilter.UniversalFilterCondition{
				Field:    "collection_id",
				Operator: searchfilter.OperatorIn,
				Value:    opt.collectionIDs,
			},
		))
	}

	if opt.toolName != "" {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithToolName(opt.toolName))
	}
	if opt.maxResults > 0 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithMaxResults(opt.maxResults))
	}
	if opt.minScore > 0 {
		frameworkOpts = append(frameworkOpts, knowledgetool.WithMinScore(opt.minScore))
	}

	return knowledgetool.NewAgenticFilterSearchTool(kb, agenticFilterInfo, frameworkOpts...)
}
