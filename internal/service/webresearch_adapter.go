package service

import (
	"context"

	"aranea-agents/internal/biz"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

// webResearchTesterAdapter adapts internal/tools/webresearch to biz.WebResearchTester.
type webResearchTesterAdapter struct{}

func (a webResearchTesterAdapter) ConfigFromSetting(provider, apiKey string, maxResults, fetchTop int, searchDepth string, timeoutSec int, httpProxy string) biz.WebResearchTestConfig {
	cfg := webresearchpkg.ConfigFromSetting(provider, apiKey, maxResults, fetchTop, searchDepth, timeoutSec, httpProxy)
	return biz.WebResearchTestConfig{
		Provider:    cfg.Provider,
		APIKey:      cfg.APIKey,
		MaxResults:  cfg.MaxResults,
		FetchTop:    cfg.FetchTop,
		SearchDepth: cfg.SearchDepth,
		TimeoutSec:  int(cfg.Timeout.Seconds()),
		HTTPProxy:   cfg.HTTPProxy,
	}
}

func (a webResearchTesterAdapter) IsReady(cfg biz.WebResearchTestConfig) bool {
	wc := a.toWebresearchConfig(cfg)
	return wc.Ready()
}

func (a webResearchTesterAdapter) TestConnection(ctx context.Context, cfg biz.WebResearchTestConfig) (biz.WebResearchTestProbeResult, error) {
	wc := a.toWebresearchConfig(cfg)
	raw, err := webresearchpkg.TestConnection(ctx, wc, loggateway.Global())
	out := biz.WebResearchTestProbeResult{
		OK:           raw.OK,
		Message:      raw.Message,
		Provider:     raw.Provider,
		ResultCount:  raw.ResultCount,
		LatencyMS:    raw.LatencyMS,
		ResponseTime: raw.ResponseTime,
	}
	return out, err
}

func (a webResearchTesterAdapter) toWebresearchConfig(cfg biz.WebResearchTestConfig) webresearchpkg.Config {
	return webresearchpkg.ConfigFromSetting(
		cfg.Provider, cfg.APIKey,
		cfg.MaxResults, cfg.FetchTop,
		cfg.SearchDepth, cfg.TimeoutSec,
		cfg.HTTPProxy,
	)
}

// ProvideWebResearchTester creates a biz.WebResearchTester backed by internal/tools/webresearch.
func ProvideWebResearchTester() biz.WebResearchTester {
	return webResearchTesterAdapter{}
}
