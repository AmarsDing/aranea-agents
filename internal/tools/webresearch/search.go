package webresearch

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Search runs a provider query with the given config (Tavily / SerpAPI).
// Uses the same outboundguard HTTP client as the web_research tool.
// FetchTop <= 0 skips page-body enrichment (search hits only).
func Search(ctx context.Context, cfg Config, query string, lg loggateway.Logger) (*SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "web_research: query is required")
	}
	// Preserve intentional zero FetchTop before defaults (defaults force FetchTop>0).
	fetchTop := cfg.FetchTop
	applyConfigDefaults(&cfg)
	cfg.FetchTop = fetchTop
	if !cfg.Ready() {
		return nil, apierror.BadRequest(apierror.DomainTool,
			"web_research: api_key is required (platform web_research settings or TAVILY_API_KEY / SERPAPI_API_KEY)")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	provider, err := newSearchProvider(cfg, lg)
	if err != nil {
		return nil, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	resp, err := provider.search(runCtx, query)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &SearchResponse{Query: query, Provider: cfg.Provider, Results: nil}, nil
	}
	if cfg.FetchTop > 0 && len(resp.Results) > 0 {
		_, _ = enrichHits(runCtx, resp.Results, cfg.FetchTop, cfg, lg)
	}
	return resp, nil
}
