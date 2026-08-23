package webresearch

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// maxContentBytesPerSource caps each fetched page. 12KB × default FetchTop
// used to dump tens of KB into the same-turn tool loop (the 250k spike).
const maxContentBytesPerSource = 2400

// researchInput is the LLM-facing schema for web_research.
type researchInput struct {
	Query string `json:"query" jsonschema:"description=Natural-language search query,required"`
}

// sourceItem is one ranked web source returned to the model.
type sourceItem struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet,omitempty"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// researchOutput is the structured tool result.
type researchOutput struct {
	Query           string       `json:"query"`
	Answer          string       `json:"answer,omitempty"`
	Sources         []sourceItem `json:"sources"`
	Provider        string       `json:"provider"`
	ResponseTimeSec float64      `json:"response_time_sec,omitempty"`
	FetchedURLCount int          `json:"fetched_url_count,omitempty"`
	Summary         string       `json:"summary"`
	Partial         bool         `json:"partial,omitempty"`
	FetchWarnings   []string     `json:"fetch_warnings,omitempty"`
}

// NewTool returns a web_research CallableTool. cfg must include a valid API key.
func NewTool(cfg Config, lg loggateway.Logger) (trpctool.CallableTool, error) {
	if !cfg.Ready() {
		return nil, apierror.BadRequest(apierror.DomainTool, "web_research: api_key is required (tool config or TAVILY_API_KEY / SERPAPI_API_KEY)")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	provider, err := newSearchProvider(cfg, lg)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, in researchInput) (researchOutput, error) {
		query := strings.TrimSpace(in.Query)
		if query == "" {
			return researchOutput{}, apierror.BadRequest(apierror.DomainTool, "web_research: query is required")
		}

		runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()

		resp, err := provider.search(runCtx, query)
		if err != nil {
			return researchOutput{}, err
		}

		hits := append([]Hit(nil), resp.Results...)
		warnings, enrichErr := enrichHits(runCtx, hits, cfg.FetchTop, cfg, lg)
		partial := enrichErr != nil || len(warnings) > 0
		if enrichErr != nil {
			warnings = append(warnings, enrichErr.Error())
		}

		sources := make([]sourceItem, 0, len(hits))
		fetched := 0
		for _, h := range hits {
			if strings.TrimSpace(h.Content) != "" {
				fetched++
			}
			sources = append(sources, sourceItem{
				Title:   h.Title,
				URL:     h.URL,
				Snippet: h.Snippet,
				Content: truncateUTF8(h.Content, maxContentBytesPerSource),
				Score:   h.Score,
			})
		}

		summary := fmt.Sprintf("Found %d sources via %s", len(sources), resp.Provider)
		if strings.TrimSpace(resp.Answer) != "" {
			summary = resp.Answer
		}

		return researchOutput{
			Query:           resp.Query,
			Answer:          resp.Answer,
			Sources:         sources,
			Provider:        resp.Provider,
			ResponseTimeSec: resp.ResponseTime,
			FetchedURLCount: fetched,
			Summary:         summary,
			Partial:         partial,
			FetchWarnings:   warnings,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("web_research"),
		function.WithDescription(
			"Search the web and return ranked sources with snippets and page excerpts in one call. "+
				"Uses Tavily or SerpAPI (platform-configured). Best for current events, documentation, comparisons, and fact-finding. "+
				"Prefer this over duckduckgo_search for general web queries.",
		),
	), nil
}
