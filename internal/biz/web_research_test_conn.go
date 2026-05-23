package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/tools/webresearch"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// WebResearchTestResult is returned by TestWebResearch.
type WebResearchTestResult struct {
	OK           bool
	Message      string
	Provider     string
	ResultCount  int
	LatencyMS    int
	ResponseTime float64
}

// TestWebResearch probes Tavily/SerpAPI using form values, falling back to stored settings and env.
func (u *SystemSettingUsecase) TestWebResearch(ctx context.Context, patch WebResearchSetting, formAPIKey string) (WebResearchTestResult, error) {
	stored, err := u.repo.GetWebResearch(ctx)
	if err != nil {
		stored = WebResearchSetting{Provider: "tavily", MaxResults: 8, FetchTop: 5, SearchDepth: "basic", TimeoutSec: 15}
	}
	merged := ApplyWebResearchPatch(stored, patch, false)
	apiKey := strings.TrimSpace(formAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(stored.APIKey)
	}
	cfg := webresearch.ConfigFromSetting(
		merged.Provider,
		apiKey,
		merged.MaxResults,
		merged.FetchTop,
		merged.SearchDepth,
		merged.TimeoutSec,
		merged.HTTPProxy,
	)
	if !cfg.Ready() {
		return WebResearchTestResult{}, kerrors.BadRequest("WEB_RESEARCH", "API key is required; save one in system settings or enter it in the test form")
	}
	raw, err := webresearch.TestConnection(ctx, cfg)
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
