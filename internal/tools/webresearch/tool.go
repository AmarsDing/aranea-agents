package webresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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
	Query            string       `json:"query"`
	Answer           string       `json:"answer,omitempty"`
	Sources          []sourceItem `json:"sources"`
	Provider         string       `json:"provider"`
	ResponseTimeSec  float64      `json:"response_time_sec,omitempty"`
	FetchedURLCount  int          `json:"fetched_url_count,omitempty"`
	Summary          string       `json:"summary"`
	Partial          bool         `json:"partial,omitempty"`
	FetchWarnings    []string     `json:"fetch_warnings,omitempty"`
}

// NewTool returns a web_research CallableTool. cfg must include a valid API key.
func NewTool(cfg Config) (trpctool.CallableTool, error) {
	if !cfg.Ready() {
		return nil, fmt.Errorf("web_research: api_key is required (tool config or TAVILY_API_KEY / SERPAPI_API_KEY)")
	}
	provider, err := newSearchProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &researchTool{cfg: cfg, provider: provider}, nil
}

type researchTool struct {
	cfg      Config
	provider searchProvider
}

func (t *researchTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "web_research",
		Description: "Search the web and return ranked sources with snippets and page excerpts in one call. " +
			"Uses Tavily or SerpAPI (platform-configured). Best for current events, documentation, comparisons, and fact-finding. " +
			"Prefer this over duckduckgo_search for general web queries.",
	}
}

func (t *researchTool) Call(ctx context.Context, args []byte) (any, error) {
	var in researchInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("web_research: invalid args: %w", err)
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, fmt.Errorf("web_research: query is required")
	}

	runCtx, cancel := context.WithTimeout(ctx, t.cfg.Timeout)
	defer cancel()

	resp, err := t.provider.search(runCtx, query)
	if err != nil {
		return nil, err
	}

	hits := append([]Hit(nil), resp.Results...)
	warnings, enrichErr := enrichHits(runCtx, hits, t.cfg.FetchTop, t.cfg)
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
			Content: truncateUTF8(h.Content, 12000),
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
