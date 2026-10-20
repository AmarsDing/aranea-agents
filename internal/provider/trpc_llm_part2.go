package provider

import (
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
	"fmt"
	"strings"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
)

func trpcModelFromCandidate(c HACandidateConfig, parentCfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
	if baseURL := strings.TrimSpace(c.BaseURL); baseURL != "" {
		if err := outboundguard.ValidateURL(baseURL); err != nil {
			lg.Warn("HA 候选模型 URL 校验失败",
				loggateway.StepID("provider.ha_candidate_url_blocked"),
				loggateway.Str("candidate_name", c.Name),
				loggateway.Str("base_url", baseURL),
				loggateway.Err(err))
			return nil, fmt.Errorf("HA candidate URL blocked: %w", err)
		}
	}

	cfg := ProviderModelConfig{
		ProviderType: strings.TrimSpace(c.ProviderType),
		BaseURL:      strings.TrimSpace(c.BaseURL),
		APIKey:       strings.TrimSpace(c.APIKey),
		ModelAPI:     strings.TrimSpace(c.Name),
		Retry:        parentCfg.Retry,
		CB:           parentCfg.CB,
		RateLimitRPM: parentCfg.RateLimitRPM,
	}
	providerName := MapProviderType(cfg.ProviderType)
	opts := buildProviderOptions(cfg, rt, lg)
	m, err := trpcprovider.Model(providerName, cfg.ModelAPI, opts...)
	if err != nil {
		lg.Error("HA candidate model 构建失败", loggateway.StepID("provider.ha_candidate_build_fail"), loggateway.Str("provider", providerName), loggateway.Str("model", cfg.ModelAPI), loggateway.Err(err))
		return nil, err
	}
	return WrapModelWithMetrics(m, cfg.ProviderType, cfg.ModelAPI), nil
}
