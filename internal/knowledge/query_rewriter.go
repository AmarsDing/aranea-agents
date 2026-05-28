package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/go-kratos/kratos/v2/errors"
)

const queryRewriteTimeout = 15 * time.Second

type RewriteStrategy string

const (
	RewriteNone          RewriteStrategy = ""
	RewriteHyDE          RewriteStrategy = "hyde"
	RewriteDecomposition RewriteStrategy = "decomposition"
	RewriteMultiQuery    RewriteStrategy = "multi_query"
)

func ParseRewriteStrategy(raw string) RewriteStrategy {
	s := RewriteStrategy(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case RewriteHyDE, RewriteDecomposition, RewriteMultiQuery:
		return s
	default:
		return RewriteNone
	}
}

type QueryRewriteResult struct {
	Queries []string
	Used    RewriteStrategy
}

type QueryRewriter struct {
	llm     biz.LLMCaller
	sys     *biz.SystemSettingUsecase
	catalog *biz.LlmProviderModelUsecase
}

func NewQueryRewriter(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase) *QueryRewriter {
	return &QueryRewriter{llm: llm, sys: sys, catalog: catalog}
}

func (r *QueryRewriter) Rewrite(ctx context.Context, query string, strategy RewriteStrategy) (*QueryRewriteResult, error) {
	if r.llm == nil {
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}
	if strategy == RewriteNone || strings.TrimSpace(query) == "" {
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}

	provider, model, err := r.resolveModel(ctx)
	if err != nil {
		event.SysLogWarn("knowledge.query_rewrite.skip", "查询重写跳过：无可用 LLM",
			event.P("error", err.Error()))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}

	rewriteCtx, cancel := context.WithTimeout(ctx, queryRewriteTimeout)
	defer cancel()

	switch strategy {
	case RewriteHyDE:
		return r.rewriteHyDE(rewriteCtx, query, provider, model)
	case RewriteDecomposition:
		return r.rewriteDecomposition(rewriteCtx, query, provider, model)
	case RewriteMultiQuery:
		return r.rewriteMultiQuery(rewriteCtx, query, provider, model)
	default:
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}
}

func (r *QueryRewriter) rewriteHyDE(ctx context.Context, query, provider, model string) (*QueryRewriteResult, error) {
	sys := `你是一名知识库检索助手。用户会提出一个问题，请你生成一个详细、信息丰富的假设性回答。这个回答将被用来在知识库中进行语义搜索，因此需要包含尽可能多的相关术语和细节。直接输出回答内容，不要任何解释或前缀。`

	text, _, err := r.llm.Call(ctx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     query,
	})
	if err != nil {
		event.SysLogWarn("knowledge.query_rewrite.hyde.fail", "HyDE 重写失败",
			event.P("error", err.Error()))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}
	return &QueryRewriteResult{Queries: []string{query, text}, Used: RewriteHyDE}, nil
}

func (r *QueryRewriter) rewriteDecomposition(ctx context.Context, query, provider, model string) (*QueryRewriteResult, error) {
	sys := `你是一名查询分解助手。用户会提出一个复杂问题，请将其分解为 2-4 个独立的子问题，每个子问题聚焦一个方面。以 JSON 数组格式输出，例如：["子问题1", "子问题2", "子问题3"]。只输出 JSON 数组，不要任何解释。`

	text, _, err := r.llm.Call(ctx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     query,
	})
	if err != nil {
		event.SysLogWarn("knowledge.query_rewrite.decomposition.fail", "查询分解失败",
			event.P("error", err.Error()))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}

	var subQueries []string
	text = strings.TrimSpace(text)
	text = stripCodeFenceJSON(text)
	if err := json.Unmarshal([]byte(text), &subQueries); err != nil || len(subQueries) == 0 {
		event.SysLogWarn("knowledge.query_rewrite.decomposition.parse_fail", "查询分解结果解析失败",
			event.P("raw", text))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}
	return &QueryRewriteResult{Queries: subQueries, Used: RewriteDecomposition}, nil
}

func (r *QueryRewriter) rewriteMultiQuery(ctx context.Context, query, provider, model string) (*QueryRewriteResult, error) {
	sys := `你是一名查询改写助手。用户会提出一个搜索查询，请生成 3 个不同角度的改写版本，以便在知识库中进行更全面的检索。以 JSON 数组格式输出改写后的查询，例如：["改写1", "改写2", "改写3"]。只输出 JSON 数组，不要任何解释。`

	text, _, err := r.llm.Call(ctx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     query,
	})
	if err != nil {
		event.SysLogWarn("knowledge.query_rewrite.multi_query.fail", "多查询改写失败",
			event.P("error", err.Error()))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}

	var variants []string
	text = strings.TrimSpace(text)
	text = stripCodeFenceJSON(text)
	if err := json.Unmarshal([]byte(text), &variants); err != nil || len(variants) == 0 {
		event.SysLogWarn("knowledge.query_rewrite.multi_query.parse_fail", "多查询改写结果解析失败",
			event.P("raw", text))
		return &QueryRewriteResult{Queries: []string{query}, Used: RewriteNone}, nil
	}

	allQueries := make([]string, 0, len(variants)+1)
	allQueries = append(allQueries, query)
	allQueries = append(allQueries, variants...)
	return &QueryRewriteResult{Queries: allQueries, Used: RewriteMultiQuery}, nil
}

func (r *QueryRewriter) resolveModel(ctx context.Context) (string, string, error) {
	if r.sys != nil {
		s, err := r.sys.Get(ctx)
		if err == nil && strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	if r.catalog != nil {
		models, err := r.catalog.List(ctx)
		if err == nil {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	return "", "", errors.ServiceUnavailable("KNOWLEDGE", "no LLM available for query rewriting; configure DefaultRefineLLM in system settings")
}

func stripCodeFenceJSON(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func dedupQueries(queries []string) []string {
	seen := make(map[string]struct{}, len(queries))
	out := make([]string, 0, len(queries))
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		out = append(out, q)
	}
	return out
}

func mergeMultiQueryResults(allChunks [][]biz.KnowledgeChunk, topK int) []biz.KnowledgeChunk {
	scoreMap := make(map[string]*biz.KnowledgeChunk)
	for _, chunks := range allChunks {
		for i := range chunks {
			ch := &chunks[i]
			key := ch.ID
			if existing, ok := scoreMap[key]; ok {
				if ch.Score > existing.Score {
					scoreMap[key] = ch
				}
			} else {
				scoreMap[key] = ch
			}
		}
	}
	all := make([]biz.KnowledgeChunk, 0, len(scoreMap))
	for _, ch := range scoreMap {
		all = append(all, *ch)
	}
	sortChunksByScoreDesc(all)
	if topK > 0 && len(all) > topK {
		all = all[:topK]
	}
	return all
}

func sortChunksByScoreDesc(chunks []biz.KnowledgeChunk) {
	for i := 1; i < len(chunks); i++ {
		for j := i; j > 0 && chunks[j].Score > chunks[j-1].Score; j-- {
			chunks[j], chunks[j-1] = chunks[j-1], chunks[j]
		}
	}
}

func fmtQueryRewriteResult(result *QueryRewriteResult) string {
	if result == nil || result.Used == RewriteNone {
		return ""
	}
	return fmt.Sprintf("strategy=%s, queries=%d", result.Used, len(result.Queries))
}
