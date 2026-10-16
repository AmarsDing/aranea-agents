package mvp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/connector/ingest/search"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

var (
	searcherMu sync.Mutex
	searcherFn search.Searcher
	wrLoader   WebResearchConfigLoader
)

// WebResearchConfigLoader returns platform/system web_research config when available.
// ok=false means fall back to env-only ConfigFromMap.
type WebResearchConfigLoader func(ctx context.Context) (cfg webresearchpkg.Config, ok bool)

// ConfigureSearcher sets the MVP search ingest Searcher (webresearch-backed or test stub).
func ConfigureSearcher(fn search.Searcher) {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	searcherFn = fn
}

// ConfigureWebResearchLoader injects platform system_settings → webresearch Config.
func ConfigureWebResearchLoader(fn WebResearchConfigLoader) {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	wrLoader = fn
}

func currentSearcher() search.Searcher {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	return searcherFn
}

func currentWebResearchLoader() WebResearchConfigLoader {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	return wrLoader
}

func resetSearcherForTest() {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	searcherFn = nil
	wrLoader = nil
}

// DefaultSearcher resolves hits via system webresearch (platform settings and/or
// TAVILY_API_KEY / SERPAPI_API_KEY). FetchTop=0: ingest only needs titles/snippets.
func DefaultSearcher() search.Searcher {
	return func(ctx context.Context, query string) ([]search.Hit, error) {
		query = strings.TrimSpace(query)
		if query == "" {
			return nil, fmt.Errorf("mvp: search query required when hits[] is empty")
		}
		cfg := webresearchpkg.ConfigFromMap(nil)
		if load := currentWebResearchLoader(); load != nil {
			if loaded, ok := load(ctx); ok && loaded.Ready() {
				cfg = loaded
			}
		}
		applyConfigDefaultsForIngest(&cfg)
		if !cfg.Ready() {
			return nil, fmt.Errorf("mvp: web_research provider unset — set TAVILY_API_KEY or SERPAPI_API_KEY, configure platform web_research, or pass hits[]")
		}
		resp, err := webresearchpkg.Search(ctx, cfg, query, loggateway.NewNoop())
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, nil
		}
		out := make([]search.Hit, 0, len(resp.Results))
		for _, h := range resp.Results {
			out = append(out, search.Hit{
				URL:     h.URL,
				Title:   h.Title,
				Snippet: firstNonEmptySnippet(h.Snippet, h.Content),
			})
		}
		return out, nil
	}
}

func applyConfigDefaultsForIngest(cfg *webresearchpkg.Config) {
	if cfg == nil {
		return
	}
	// Ingest does not need page body enrichment.
	cfg.FetchTop = 0
	cfg.IncludeRawContent = false
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 8
	}
}

func firstNonEmptySnippet(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// EnsureDefaultSearcher installs DefaultSearcher when none is configured.
func EnsureDefaultSearcher() {
	searcherMu.Lock()
	defer searcherMu.Unlock()
	if searcherFn == nil {
		searcherFn = DefaultSearcher()
	}
}
