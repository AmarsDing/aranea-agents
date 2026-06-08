package provider

import (
	"context"
	"fmt"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
	"strings"
	"time"

	"google.golang.org/genai"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcanthropic "trpc.group/trpc-go/trpc-agent-go/model/anthropic"
	trpcfailover "trpc.group/trpc-go/trpc-agent-go/model/failover"
	trpcgemini "trpc.group/trpc-go/trpc-agent-go/model/gemini"
	trpchedge "trpc.group/trpc-go/trpc-agent-go/model/hedge"
	trpchunyuan "trpc.group/trpc-go/trpc-agent-go/model/hunyuan"
	trpcollama "trpc.group/trpc-go/trpc-agent-go/model/ollama"
	trpcopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
)

func TRPCModelForProviderModel(ctx context.Context, catalog biz.TeamModelCatalog, rt *RoundTrip, prov, modelAPI string, lg loggateway.Logger) (trpcmodel.Model, error) {
	if catalog == nil {
		return nil, ErrNilLlmCatalog
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
	if err != nil {
		lg.Error("模型目录查询失败", loggateway.StepID("provider.catalog_fail"), loggateway.Str("provider", prov), loggateway.Str("model", modelAPI), loggateway.Err(err))
		return nil, err
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{Model: pm.Model, ConfigJSON: pm.ConfigJSON})
	if err != nil {
		lg.Error("模型目录配置解析失败", loggateway.StepID("provider.catalog_parse_fail"), loggateway.Str("provider", prov), loggateway.Str("model", modelAPI), loggateway.Err(err))
		return nil, err
	}
	cfg = MergeModelConfig(cfg, pm.ConfigJSON)
	lg.Info("模型配置已解析", loggateway.StepID("provider.config_resolved"), loggateway.Phase("done"),
		loggateway.Str("provider", prov), loggateway.Str("model", modelAPI), loggateway.Str("provider_type", cfg.ProviderType), loggateway.Str("ha_mode", cfg.HAMode))
	return trpcModelFromProviderModelConfig(ctx, cfg, rt, lg)
}

func trpcModelFromProviderModelConfig(ctx context.Context, cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
	name := strings.TrimSpace(cfg.ModelAPI)
	if name == "" {
		return nil, ErrNilLlmCatalog
	}

	// Preflight connectivity check: verify the LLM API endpoint is reachable
	// before constructing the full model, so the user gets a fast failure
	// instead of a hanging request.
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		if err := outboundguard.ValidateURL(baseURL); err != nil {
			return nil, fmt.Errorf("LLM API URL blocked: %w", err)
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		probeReq, err := http.NewRequestWithContext(probeCtx, http.MethodHead, baseURL, nil)
		if err == nil {
			client := outboundguard.NewClient(15 * time.Second)
			resp, err := client.Do(probeReq)
			if err != nil {
				lg.Error("模型 API 预检失败", loggateway.StepID("provider.preflight_fail"), loggateway.Str("url", baseURL), loggateway.Err(err))
				return nil, fmt.Errorf("LLM API unreachable (%s): %w", baseURL, err)
			}
			resp.Body.Close()
			lg.Info("模型 API 预检通过", loggateway.StepID("provider.preflight_ok"), loggateway.Phase("done"), loggateway.Str("url", baseURL), loggateway.Int("status", resp.StatusCode))
		}
	}

	providerName := MapProviderType(cfg.ProviderType)
	opts := buildProviderOptions(cfg, rt)

	m, err := trpcprovider.Model(providerName, name, opts...)
	if err != nil {
		lg.Error("Provider model 构建失败", loggateway.StepID("provider.build_fail"), loggateway.Str("provider", providerName), loggateway.Str("model", name), loggateway.Err(err))
		return nil, err
	}
	m = WrapModelWithMetrics(m, strings.TrimSpace(cfg.ProviderType), name)

	return wrapHA(m, cfg, rt, lg)
}

func MapProviderType(pt string) string {
	switch strings.ToLower(strings.TrimSpace(pt)) {
	case "anthropic":
		return "anthropic"
	case "gemini", "google gemini":
		return "gemini"
	case "ollama":
		return "ollama"
	case "hunyuan":
		return "hunyuan"
	case "huggingface":
		return "huggingface"
	case "bedrock":
		return "bedrock"
	default:
		return "openai"
	}
}

func mapVariant(pt string) string {
	switch strings.ToLower(strings.TrimSpace(pt)) {
	case "deepseek":
		return "deepseek"
	case "qwen":
		return "qwen"
	case "hunyuan":
		return "hunyuan"
	default:
		return ""
	}
}

func InferVariant(cfg ProviderModelConfig) string {
	if v := mapVariant(cfg.ProviderType); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg.Variant); v != "" {
		return v
	}
	return mapVariantFromBaseURL(cfg.BaseURL)
}

func mapVariantFromBaseURL(baseURL string) string {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	if u == "" {
		return ""
	}
	if strings.Contains(u, "deepseek.com") {
		return "deepseek"
	}
	return ""
}

func ModelSupportsImageAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string) bool {
	if catalog == nil {
		return !looksLikeDeepSeek(prov, model)
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(model))
	if err != nil {
		return !looksLikeDeepSeek(prov, model)
	}
	if pm.CapabilitiesExplicit {
		return pm.Capabilities.Vision && !pm.Capabilities.TextOnly
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      pm.Model,
		ConfigJSON: pm.ConfigJSON,
	})
	if err == nil {
		cfg = MergeModelConfig(cfg, pm.ConfigJSON)
		if hasExplicitCapabilities(cfg.Capabilities) {
			return cfg.Capabilities.Vision && !cfg.Capabilities.TextOnly
		}
		if InferVariant(cfg) == "deepseek" {
			return false
		}
	}
	return !looksLikeDeepSeek(pm.Provider, pm.Model)
}

func ModelSupportsFileAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string) bool {
	if catalog == nil {
		return true
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(model))
	if err != nil {
		return true
	}
	if pm.CapabilitiesExplicit {
		return pm.Capabilities.File
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      pm.Model,
		ConfigJSON: pm.ConfigJSON,
	})
	if err == nil {
		cfg = MergeModelConfig(cfg, pm.ConfigJSON)
		if hasExplicitCapabilities(cfg.Capabilities) {
			return cfg.Capabilities.File
		}
	}
	return true
}

func CapabilitiesForProviderModel(pm biz.ProviderModel) biz.ModelCapabilities {
	if pm.CapabilitiesExplicit {
		return pm.Capabilities
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      pm.Model,
		ConfigJSON: pm.ConfigJSON,
	})
	if err == nil {
		cfg = MergeModelConfig(cfg, pm.ConfigJSON)
	}
	caps := cfg.Capabilities
	if !hasExplicitCapabilities(caps) {
		caps.Text = true
		caps.File = true
		caps.ToolCall = true
	}
	if InferVariant(cfg) == "deepseek" || looksLikeDeepSeek(pm.Provider, pm.Model) {
		caps.Text = true
		caps.TextOnly = true
		caps.Vision = false
		caps.Audio = false
	}
	if caps.Cache || cfg.OptimizeForCache || cfg.CacheSystemPrompt || cfg.CacheTools || cfg.CacheMessages {
		caps.Cache = true
	}
	if caps.Thinking || cfg.ReasoningBackfill {
		caps.Thinking = true
	}
	return caps
}

func looksLikeDeepSeek(parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(strings.TrimSpace(part)), "deepseek") {
			return true
		}
	}
	return false
}

func buildProviderOptions(cfg ProviderModelConfig, rt *RoundTrip) []trpcprovider.Option {
	var opts []trpcprovider.Option

	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		opts = append(opts, trpcprovider.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		opts = append(opts, trpcprovider.WithBaseURL(baseURL))
	}
	if v := InferVariant(cfg); v != "" {
		opts = append(opts, trpcprovider.WithVariant(v))
	}
	if cfg.ChannelBufferSize > 0 {
		opts = append(opts, trpcprovider.WithChannelBufferSize(cfg.ChannelBufferSize))
	}
	if cfg.EnableTokenTailoring {
		opts = append(opts, trpcprovider.WithEnableTokenTailoring(true))
	}
	if cfg.MaxInputTokens > 0 {
		opts = append(opts, trpcprovider.WithMaxInputTokens(cfg.MaxInputTokens))
	}
	transport := http.DefaultTransport
	if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
		transport = rt.HTTP.Transport
	}
	if cfg.RateLimitRPM > 0 {
		transport = wrapRateLimitTransport(transport, cfg.RateLimitRPM)
	}
	if transport != nil {
		opts = append(opts, trpcprovider.WithHTTPClientTransport(transport))
	}

	opts = append(opts, buildOpenAISpecificOptions(cfg)...)
	opts = append(opts, buildAnthropicSpecificOptions(cfg)...)
	opts = append(opts, buildGeminiSpecificOptions(cfg, rt)...)
	opts = append(opts, buildOllamaSpecificOptions(cfg)...)
	opts = append(opts, buildHunyuanSpecificOptions(cfg)...)
	opts = append(opts, buildHuggingFaceSpecificOptions(cfg)...)
	opts = append(opts, buildBedrockSpecificOptions(cfg)...)

	return opts
}

func buildHuggingFaceSpecificOptions(_ ProviderModelConfig) []trpcprovider.Option {
	return nil
}

func buildBedrockSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option {
	if strings.ToLower(cfg.ProviderType) != "bedrock" {
		return nil
	}
	region := strings.TrimSpace(cfg.AWSRegion)
	if region == "" {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithExtraFields(map[string]any{"aws_region": region})}
}

func buildOpenAISpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option {
	var providerOpts []trpcopenai.Option
	if cfg.OptimizeForCache {
		providerOpts = append(providerOpts, trpcopenai.WithOptimizeForCache(true))
	}
	if cfg.ReasoningBackfill {
		providerOpts = append(providerOpts, trpcopenai.WithReasoningContentBackfill(true))
	}
	if cfg.ShowToolCallDelta {
		providerOpts = append(providerOpts, trpcopenai.WithShowToolCallDelta(true))
	}
	if cfg.ContextWindow > 0 {
		providerOpts = append(providerOpts, trpcopenai.WithContextWindow(cfg.ContextWindow))
	}
	if len(providerOpts) == 0 {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithOpenAIOption(providerOpts...)}
}

func buildAnthropicSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option {
	var providerOpts []trpcanthropic.Option
	if cfg.CacheSystemPrompt {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPrompt(true))
		// Enable dual-breakpoint mode for system prompt caching.
		// Breakpoint 1: end of TextBlock[0] (static layer: identity + instructions + skills + staticRuntimeCue)
		// Breakpoint 2: end of TextBlock[2] (semi-static layer: dynamicRuntimeCue + SkillGuidance)
		providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPromptDualBreakpoint(2))
	}
	if cfg.CacheTools {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheTools(true))
	}
	if cfg.CacheMessages {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheMessages(true))
	}
	if cfg.ShowToolCallDelta {
		providerOpts = append(providerOpts, trpcanthropic.WithShowToolCallDelta(true))
	}
	if len(providerOpts) == 0 {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithAnthropicOption(providerOpts...)}
}

func buildGeminiSpecificOptions(cfg ProviderModelConfig, rt *RoundTrip) []trpcprovider.Option {
	var providerOpts []trpcgemini.Option
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey != "" || (rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil) {
		gcc := &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendVertexAI,
		}
		if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
			gcc.HTTPClient = &http.Client{Transport: rt.HTTP.Transport}
		}
		providerOpts = append(providerOpts, trpcgemini.WithGeminiClientConfig(gcc))
	}
	if cfg.ContextWindow > 0 {
		providerOpts = append(providerOpts, trpcgemini.WithMaxInputTokens(cfg.ContextWindow))
	}
	if len(providerOpts) == 0 {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithGeminiOption(providerOpts...)}
}

func buildOllamaSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option {
	var providerOpts []trpcollama.Option
	if cfg.KeepAliveMinutes > 0 {
		providerOpts = append(providerOpts, trpcollama.WithKeepAlive(time.Duration(cfg.KeepAliveMinutes)*time.Minute))
	}
	if cfg.ContextWindow > 0 {
		providerOpts = append(providerOpts, trpcollama.WithMaxInputTokens(cfg.ContextWindow))
	}
	if len(providerOpts) == 0 {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithOllamaOption(providerOpts...)}
}

func buildHunyuanSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option {
	var providerOpts []trpchunyuan.Option
	if secretID := strings.TrimSpace(cfg.SecretID); secretID != "" {
		providerOpts = append(providerOpts, trpchunyuan.WithSecretId(secretID))
	}
	if secretKey := strings.TrimSpace(cfg.SecretKey); secretKey != "" {
		providerOpts = append(providerOpts, trpchunyuan.WithSecretKey(secretKey))
	}
	if cfg.ContextWindow > 0 {
		providerOpts = append(providerOpts, trpchunyuan.WithContextWindow(cfg.ContextWindow))
	}
	if len(providerOpts) == 0 {
		return nil
	}
	return []trpcprovider.Option{trpcprovider.WithHunyuanOption(providerOpts...)}
}

func wrapHA(primary trpcmodel.Model, cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.HAMode)) {
	case "failover":
		return wrapFailover(cfg, rt, primary, lg)
	case "hedge":
		return wrapHedge(cfg, rt, primary, lg)
	}
	return primary, nil
}

func wrapFailover(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HACandidates {
		m, err := trpcModelFromCandidate(c, rt, lg)
		if err != nil {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) < 2 {
		lg.Warn("HA failover 候选不足，回退到主模型", loggateway.StepID("provider.ha_failover_fallback"), loggateway.Str("ha_mode", "failover"), loggateway.Int("candidates", len(candidates)))
		return primary, nil
	}
	fo, err := trpcfailover.New(
		trpcfailover.WithCandidates(candidates...),
		trpcfailover.WithSwitchCallback(func(ctx context.Context, from, to string, err error) {
			lg.Warn("HA 故障切换",
				loggateway.StepID("provider.ha_failover"),
				loggateway.Str("ha_mode", "failover"),
				loggateway.Str("from_candidate", from),
				loggateway.Str("to_candidate", to),
				loggateway.Err(err),
			)
		}),
	)
	if err != nil {
		lg.Warn("HA failover 构建失败，回退到主模型", loggateway.StepID("provider.ha_failover_build_fail"), loggateway.Err(err))
		return primary, nil
	}
	return fo, nil
}

func wrapHedge(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HACandidates {
		m, err := trpcModelFromCandidate(c, rt, lg)
		if err != nil {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) < 2 {
		lg.Warn("HA hedge 候选不足，回退到主模型", loggateway.StepID("provider.ha_hedge_fallback"), loggateway.Str("ha_mode", "hedge"), loggateway.Int("candidates", len(candidates)))
		return primary, nil
	}
	hedgeOpts := []trpchedge.Option{
		trpchedge.WithCandidates(candidates...),
		trpchedge.WithSwitchCallback(func(ctx context.Context, from, to string, err error) {
			lg.Warn("HA 对冲切换",
				loggateway.StepID("provider.ha_hedge"),
				loggateway.Str("ha_mode", "hedge"),
				loggateway.Str("primary_candidate", from),
				loggateway.Str("winner_candidate", to),
				loggateway.Err(err),
			)
		}),
	}
	if cfg.HAHedgeDelayMs > 0 {
		hedgeOpts = append(hedgeOpts, trpchedge.WithDelay(time.Duration(cfg.HAHedgeDelayMs)*time.Millisecond))
	}
	h, err := trpchedge.New(hedgeOpts...)
	if err != nil {
		lg.Warn("HA hedge 构建失败，回退到主模型", loggateway.StepID("provider.ha_hedge_build_fail"), loggateway.Err(err))
		return primary, nil
	}
	return h, nil
}

func trpcModelFromCandidate(c HACandidateConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
	if baseURL := strings.TrimSpace(c.BaseURL); baseURL != "" {
		if err := outboundguard.ValidateURL(baseURL); err != nil {
			return nil, fmt.Errorf("HA candidate URL blocked: %w", err)
		}
	}
	providerName := MapProviderType(c.ProviderType)
	opts := []trpcprovider.Option{}
	if apiKey := strings.TrimSpace(c.APIKey); apiKey != "" {
		opts = append(opts, trpcprovider.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(c.BaseURL); baseURL != "" {
		opts = append(opts, trpcprovider.WithBaseURL(baseURL))
	}
	if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
		opts = append(opts, trpcprovider.WithHTTPClientTransport(rt.HTTP.Transport))
	}
	m, err := trpcprovider.Model(providerName, c.Name, opts...)
	if err != nil {
		lg.Error("HA candidate model 构建失败", loggateway.StepID("provider.ha_candidate_build_fail"), loggateway.Str("provider", providerName), loggateway.Str("model", c.Name), loggateway.Err(err))
		return nil, err
	}
	return WrapModelWithMetrics(m, strings.TrimSpace(c.ProviderType), c.Name), nil
}
