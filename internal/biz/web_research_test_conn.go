package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// WebResearchTestConfig holds the configuration for a web research test probe.
type WebResearchTestConfig struct {
	Provider    string
	APIKey      string
	MaxResults  int
	FetchTop    int
	SearchDepth string
	TimeoutSec  int
	HTTPProxy   string
}

// WebResearchTestProbeResult is the raw result from a web research test probe.
type WebResearchTestProbeResult struct {
	OK           bool
	Message      string
	Provider     string
	ResultCount  int
	LatencyMS    int
	ResponseTime float64
}

// WebResearchTestResult is returned by SystemSettingUsecase.TestWebResearch.
type WebResearchTestResult = WebResearchTestProbeResult

// WebResearchTester abstracts web research connectivity testing so biz does not
// depend on internal/tools/webresearch directly.
type WebResearchTester interface {
	ConfigFromSetting(provider, apiKey string, maxResults, fetchTop int, searchDepth string, timeoutSec int, httpProxy string) WebResearchTestConfig
	IsReady(cfg WebResearchTestConfig) bool
	TestConnection(ctx context.Context, cfg WebResearchTestConfig) (WebResearchTestProbeResult, error)
}

// TestWebResearch probes Tavily/SerpAPI using form values, falling back to stored settings and env.
func (u *SystemSettingUsecase) TestWebResearch(ctx context.Context, patch WebResearchSetting, formAPIKey string) (WebResearchTestResult, error) {
	if u.webResearchTester == nil {
		return WebResearchTestResult{}, apierror.BadRequest("WEB_RESEARCH", "web research tester not configured")
	}
	stored, err := u.repo.GetWebResearch(ctx)
	if err != nil {
		stored = WebResearchSetting{Provider: "tavily", MaxResults: 8, FetchTop: 5, SearchDepth: "basic", TimeoutSec: 15}
	}
	merged := ApplyWebResearchPatch(stored, patch, false)
	apiKey := strings.TrimSpace(formAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(stored.APIKey)
	}
	cfg := u.webResearchTester.ConfigFromSetting(
		merged.Provider,
		apiKey,
		merged.MaxResults,
		merged.FetchTop,
		merged.SearchDepth,
		merged.TimeoutSec,
		merged.HTTPProxy,
	)
	if !u.webResearchTester.IsReady(cfg) {
		return WebResearchTestResult{}, apierror.BadRequest("WEB_RESEARCH", "API key is required; save one in system settings or enter it in the test form")
	}
	raw, err := u.webResearchTester.TestConnection(ctx, cfg)
	out := WebResearchTestResult{
		OK:           raw.OK,
		Message:      raw.Message,
		Provider:     raw.Provider,
		ResultCount:  raw.ResultCount,
		LatencyMS:    raw.LatencyMS,
		ResponseTime: raw.ResponseTime,
	}
	return out, err
}
