package provider

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"

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

func TRPCModelForProviderModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *RoundTrip, prov, modelAPI string) (trpcmodel.Model, error) {
	if catalog == nil {
		return nil, ErrNilLlmCatalog
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
	if err != nil {
		return nil, err
	}
	cfg, err := CatalogFromProviderModel(pm)
	if err != nil {
		return nil, err
	}
	cfg = MergeCatalogConfig(cfg, pm.ConfigJSON)
	return trpcModelFromCatalogConfig(ctx, cfg, rt)
}

func trpcModelFromCatalogConfig(ctx context.Context, cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
	name := strings.TrimSpace(cfg.ModelAPI)
	if name == "" {
		return nil, ErrNilLlmCatalog
	}

	providerName := MapProviderType(cfg.ProviderType)
	opts := buildProviderOptions(cfg, rt)

	m, err := trpcprovider.Model(providerName, name, opts...)
	if err != nil {
		return nil, err
	}

	return wrapHA(ctx, m, cfg, rt)
}

func MapProviderType(pt string) string {
	switch strings.ToLower(strings.TrimSpace(pt)) {
	case "anthropic":
		return "anthropic"
	case "gemini":
		return "gemini"
	case "ollama":
		return "ollama"
	case "hunyuan":
		return "hunyuan"
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

func buildProviderOptions(cfg CatalogConfig, rt *RoundTrip) []trpcprovider.Option {
	var opts []trpcprovider.Option

	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		opts = append(opts, trpcprovider.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		opts = append(opts, trpcprovider.WithBaseURL(baseURL))
	}
	if v := mapVariant(cfg.ProviderType); v != "" {
		opts = append(opts, trpcprovider.WithVariant(v))
	} else if v := strings.TrimSpace(cfg.Variant); v != "" {
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
	if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
		opts = append(opts, trpcprovider.WithHTTPClientTransport(rt.HTTP.Transport))
	}

	opts = append(opts, buildOpenAISpecificOptions(cfg)...)
	opts = append(opts, buildAnthropicSpecificOptions(cfg)...)
	opts = append(opts, buildGeminiSpecificOptions(cfg, rt)...)
	opts = append(opts, buildOllamaSpecificOptions(cfg)...)
	opts = append(opts, buildHunyuanSpecificOptions(cfg)...)

	return opts
}

func buildOpenAISpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
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

func buildAnthropicSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
	var providerOpts []trpcanthropic.Option
	if cfg.CacheSystemPrompt {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPrompt(true))
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

func buildGeminiSpecificOptions(cfg CatalogConfig, rt *RoundTrip) []trpcprovider.Option {
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

func buildOllamaSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
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

func buildHunyuanSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
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

func wrapHA(ctx context.Context, primary trpcmodel.Model, cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
	_ = ctx
	switch strings.ToLower(strings.TrimSpace(cfg.HAMode)) {
	case "failover":
		return wrapFailover(cfg, rt, primary)
	case "hedge":
		return wrapHedge(cfg, rt, primary)
	}
	return primary, nil
}

func wrapFailover(cfg CatalogConfig, rt *RoundTrip, primary trpcmodel.Model) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HACandidates {
		m, err := trpcModelFromCandidate(c, rt)
		if err != nil {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) < 2 {
		return primary, nil
	}
	fo, err := trpcfailover.New(trpcfailover.WithCandidates(candidates...))
	if err != nil {
		return primary, nil
	}
	return fo, nil
}

func wrapHedge(cfg CatalogConfig, rt *RoundTrip, primary trpcmodel.Model) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HACandidates {
		m, err := trpcModelFromCandidate(c, rt)
		if err != nil {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) < 2 {
		return primary, nil
	}
	hedgeOpts := []trpchedge.Option{trpchedge.WithCandidates(candidates...)}
	if cfg.HAHedgeDelayMs > 0 {
		hedgeOpts = append(hedgeOpts, trpchedge.WithDelay(time.Duration(cfg.HAHedgeDelayMs)*time.Millisecond))
	}
	h, err := trpchedge.New(hedgeOpts...)
	if err != nil {
		return primary, nil
	}
	return h, nil
}

func trpcModelFromCandidate(c HACandidateConfig, rt *RoundTrip) (trpcmodel.Model, error) {
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
	return trpcprovider.Model(providerName, c.Name, opts...)
}
