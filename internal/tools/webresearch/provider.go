package webresearch

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// Hit is a normalized search result used by web_research.
type Hit struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet,omitempty"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// SearchResponse is the normalized output from a search provider.
type SearchResponse struct {
	Query        string  `json:"query"`
	Answer       string  `json:"answer,omitempty"`
	Results      []Hit   `json:"results"`
	Provider     string  `json:"provider"`
	ResponseTime float64 `json:"response_time_sec,omitempty"`
}

type searchProvider interface {
	search(ctx context.Context, query string) (*SearchResponse, error)
}

func newSearchProvider(cfg Config, lg loggateway.Logger) (searchProvider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ProviderSerpAPI:
		return newSerpAPIProvider(cfg, lg)
	case ProviderTavily, "":
		return newTavilyProvider(cfg, lg)
	default:
		return nil, fmt.Errorf("web_research: unsupported provider %q (use tavily or serpapi)", cfg.Provider)
	}
}
