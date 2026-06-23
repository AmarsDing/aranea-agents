package webresearch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const testSearchQuery = "Aranea web research connectivity test"

// TestResult is the outcome of a provider connectivity probe.
type TestResult struct {
	OK           bool    `json:"ok"`
	Message      string  `json:"message"`
	Provider     string  `json:"provider"`
	ResultCount  int     `json:"result_count"`
	LatencyMS    int     `json:"latency_ms"`
	ResponseTime float64 `json:"response_time_sec,omitempty"`
}

// ConfigFromSetting builds runtime Config from platform fields and resolved API key.
func ConfigFromSetting(provider string, apiKey string, maxResults, fetchTop int, searchDepth string, timeoutSec int, httpProxy string) Config {
	m := map[string]any{
		"provider":     provider,
		"api_key":      apiKey,
		"search_depth": searchDepth,
		"http_proxy":   httpProxy,
	}
	if maxResults > 0 {
		m["max_results"] = maxResults
	}
	if fetchTop > 0 {
		m["fetch_top"] = fetchTop
	}
	if timeoutSec > 0 {
		m["timeout_sec"] = timeoutSec
	}
	cfg := ConfigFromMap(m)
	cfg.MaxResults = 1
	cfg.FetchTop = 0
	cfg.IncludeAnswer = false
	cfg.IncludeRawContent = false
	return cfg
}

// TestConnection runs a minimal search against the configured provider.
func TestConnection(ctx context.Context, cfg Config, lg loggateway.Logger) (TestResult, error) {
	if !cfg.Ready() {
		return TestResult{}, apierror.BadRequest(apierror.DomainTool, "web_research: api_key is required")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	provider, err := newSearchProvider(cfg, lg)
	if err != nil {
		return TestResult{Provider: cfg.Provider, Message: err.Error()}, err
	}
	started := time.Now()
	resp, err := provider.search(ctx, testSearchQuery)
	latency := int(time.Since(started).Milliseconds())
	out := TestResult{
		Provider:  strings.TrimSpace(cfg.Provider),
		LatencyMS: latency,
	}
	if err != nil {
		out.Message = err.Error()
		return out, err
	}
	count := len(resp.Results)
	out.OK = true
	out.ResultCount = count
	out.ResponseTime = resp.ResponseTime
	out.Message = fmt.Sprintf("%s search OK (%d results, %d ms)", out.Provider, count, latency)
	return out, nil
}
