package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"

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

// TRPCModelForProviderModel resolves a trpc-agent-go model.Model from the biz
// catalog and wires it with the project's HTTP transport (rate-limit, retry,
// circuit-breaker, metrics).
//
// Per-task-type timeout:
// The HTTP client's base timeout is set at wire time via TimeoutPolicy
// (see provideLLMHTTPClient in cmd/admin/wire.go, using TaskTypeModerate as
// the baseline). To apply a different timeout for a specific call, set the
// TaskType in the request context before invoking model.GenerateContent:
//
//	ctx = provider.WithTaskType(ctx, provider.TaskTypeCodeGen)
//	ctx, cancel, _ := timeoutPolicy.ApplyTimeoutFromCtx(ctx)
//	if cancel != nil { defer cancel() }
//	resp, err := model.GenerateContent(ctx, req)
//
// When no TaskType is set in context, the http.Client's default timeout
// (TaskTypeModerate = 60min) applies.
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
	lg.Info("模型配置已解析", loggateway.StepID("provider.config_resolved"), loggateway.Phase("done"),
		loggateway.Str("provider", prov), loggateway.Str("model", modelAPI), loggateway.Str("provider_type", cfg.ProviderType), loggateway.Str("ha_mode", cfg.HA.Mode))
	return trpcModelFromProviderModelConfig(ctx, cfg, rt, lg)
}

func trpcModelFromProviderModelConfig(ctx context.Context, cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
	name := strings.TrimSpace(cfg.ModelAPI)
	if name == "" {
		return nil, ErrEmptyModelAPI
	}

	// Security validation is still required, but the synchronous HTTP HEAD
	// preflight has been removed — it added up to 10s latency on the
	// critical path (Agent build → SetRunStatus). The LLM call itself
	// validates connectivity; a preflight is redundant.
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		if err := outboundguard.ValidateURL(baseURL); err != nil {
			return nil, fmt.Errorf("LLM API URL blocked: %w", err)
		}
	}

	providerName := MapProviderType(cfg.ProviderType)
	opts := buildProviderOptions(cfg, rt, lg)

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

// ModelSupportsImageAttachments tells the caller whether the model can ingest
// image attachments. Honours explicit CapabilitiesExplicit first, then derives
// from config_json, then falls back to a name-based heuristic.
// When the catalog is unavailable, it falls back to a conservative heuristic
// (allow all except DeepSeek-like models). When the catalog returns a real
// error (DB failure, parse error), it logs a warning before falling back.
//
// 默认策略说明（与 CapabilitiesForProviderModel 的差异）：
//   - 本函数用于运行时附件校验，采用"保守允许"策略——无显式能力声明时
//     默认允许图片附件（除非启发式判定为 DeepSeek），避免阻断用户正常使用。
//   - CapabilitiesForProviderModel 用于能力集展示，采用"保守不声明"策略——
//     无显式能力声明时 Vision 默认 false，避免错误声明能力。
//   - 两种策略服务于不同场景，差异是有意为之，非 bug。
func ModelSupportsImageAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string, lg loggateway.Logger) bool {
	cfg, err := resolveCapabilities(ctx, catalog, prov, model)
	if err != nil {
		lg.Warn("resolve capabilities failed, falling back to heuristic",
			loggateway.StepID("provider.capability_fallback"),
			loggateway.Str("provider", prov),
			loggateway.Str("model", model),
			loggateway.Err(err))
		return !looksLikeDeepSeek(prov, model)
	}
	if cfg == nil {
		return !looksLikeDeepSeek(prov, model)
	}
	if hasExplicitCapabilities(cfg.Capabilities) {
		return cfg.Capabilities.Vision && !cfg.Capabilities.TextOnly
	}
	if InferVariant(*cfg) == "deepseek" {
		return false
	}
	return !looksLikeDeepSeek(prov, model)
}

// ModelSupportsFileAttachments tells the caller whether the model can ingest
// arbitrary file attachments.
// When the catalog is unavailable, it defaults to true (conservative: allow
// file attachments unless explicitly disabled). When the catalog returns a
// real error, it logs a warning before falling back.
func ModelSupportsFileAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string, lg loggateway.Logger) bool {
	cfg, err := resolveCapabilities(ctx, catalog, prov, model)
	if err != nil {
		lg.Warn("resolve capabilities failed, falling back to heuristic",
			loggateway.StepID("provider.capability_fallback"),
			loggateway.Str("provider", prov),
			loggateway.Str("model", model),
			loggateway.Err(err))
		return true
	}
	if cfg == nil {
		return true
	}
	if hasExplicitCapabilities(cfg.Capabilities) {
		return cfg.Capabilities.File
	}
	return true
}

// CapabilitiesForProviderModel returns the effective capability set for a
// provider-model row. Explicit values win; otherwise we derive from the
// variant (DeepSeek forces TextOnly) and from caching/thinking flags.
// When config_json parsing fails, it returns a conservative default
// (Text + File + ToolCall).
func CapabilitiesForProviderModel(pm biz.ProviderModel) biz.ModelCapabilities {
	if pm.CapabilitiesExplicit {
		return pm.Capabilities
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      pm.Model,
		ConfigJSON: pm.ConfigJSON,
	})
	if err != nil {
		// Config parsing failed — return conservative defaults rather than
		// silently producing a wrong capability set.
		return biz.ModelCapabilities{Text: true, File: true, ToolCall: true}
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
	if caps.Cache || cfg.OptimizeForCache || cfg.Cache.SystemPrompt || cfg.Cache.Tools || cfg.Cache.Messages {
		caps.Cache = true
	}
	if caps.Thinking || cfg.ReasoningBackfill {
		caps.Thinking = true
	}
	return caps
}

// resolveCapabilities is the shared helper used by ModelSupports{Image,File}Attachments.
// It returns nil config when the model is not found in the catalog (a legitimate
// "not found" case), and returns a non-nil error for database or parsing failures
// that the caller should know about.
func resolveCapabilities(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string) (*ProviderModelConfig, error) {
	if catalog == nil {
		return nil, nil
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(model))
	if err != nil {
		// Distinguish "not found" from real errors.
		if errors.Is(err, biz.ErrProviderModelNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("provider: resolve capabilities: %w", err)
	}
	if pm.CapabilitiesExplicit {
		cfg := ProviderModelConfig{Capabilities: pm.Capabilities}
		return &cfg, nil
	}
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      pm.Model,
		ConfigJSON: pm.ConfigJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("provider: resolve capabilities: %w", err)
	}
	return &cfg, nil
}

func looksLikeDeepSeek(parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(strings.TrimSpace(part)), "deepseek") {
			return true
		}
	}
	return false
}

func buildProviderOptions(cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) []trpcprovider.Option {
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
		// Map TokenTailoringStrategy to the corresponding framework TailoringStrategy implementation.
		if strategy := resolveTailoringStrategy(cfg.TokenTailoringStrategy); strategy != nil {
			opts = append(opts, trpcprovider.WithTailoringStrategy(strategy))
		}
		// Map TokenTailoringSafetyMargin to framework TokenTailoringConfig.SafetyMarginRatio.
		if cfg.TokenTailoringSafetyMargin > 0 {
			opts = append(opts, trpcprovider.WithTokenTailoringConfig(&trpcmodel.TokenTailoringConfig{
				SafetyMarginRatio: cfg.TokenTailoringSafetyMargin,
			}))
		}
	}
	if cfg.MaxInputTokens > 0 {
		opts = append(opts, trpcprovider.WithMaxInputTokens(cfg.MaxInputTokens))
	}
	// Only set a custom transport when we have a reason to override the
	// framework default: either the caller provides one (outboundguard,
	// custom TLS) or we need to wrap it with rate-limiting / retry / circuit-breaker.
	transport := http.DefaultTransport
	hasCustomTransport := rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil
	if hasCustomTransport {
		transport = rt.HTTP.Transport
	}
	// 透传 rt.HTTP.Timeout：框架 DefaultNewHTTPClient 创建客户端时不设置 Timeout，
	// 这里通过 timeoutTransport 在 transport 层注入超时，避免 LLM 调用无限挂起。
	// 参考：wire.go 通过 http.Client.Timeout 设置 60min 基线超时。
	if rt != nil && rt.HTTP != nil && rt.HTTP.Timeout > 0 {
		transport = newTimeoutTransport(transport, rt.HTTP.Timeout)
		hasCustomTransport = true // 强制传给框架，否则框架用默认客户端（无超时）
	}
	if cfg.RateLimitRPM > 0 {
		transport = wrapRateLimitTransport(transport, cfg.RateLimitRPM)
	}
	if cfg.Retry.MaxAttempts != 0 {
		baseDelay := time.Duration(cfg.Retry.BaseDelayMs) * time.Millisecond
		if baseDelay <= 0 {
			baseDelay = 1000 * time.Millisecond
		}
		maxDelay := time.Duration(cfg.Retry.MaxDelayMs) * time.Millisecond
		if maxDelay <= 0 {
			maxDelay = 30000 * time.Millisecond
		}
		var onRetry RetryCallback
		if rt != nil {
			onRetry = rt.OnRetry
		}
		transport = newRetryTransport(transport, cfg.Retry.MaxAttempts, baseDelay, maxDelay, lg, onRetry)
	}
	if cfg.CB.Enabled {
		cbCfg := biztool.CircuitBreakerConfig{
			FailureThreshold:   cfg.CB.FailureThreshold,
			RecoveryTimeoutSec: cfg.CB.RecoverySec,
		}
		cb := biztool.NewCircuitBreaker(fmt.Sprintf("provider:%s:%s", cfg.ProviderType, cfg.ModelAPI), cbCfg)
		transport = newCircuitBreakerTransport(transport, cb, lg)
	}
	// Usage tap (outermost): normalize DeepSeek-style prompt_cache_hit_tokens
	// into OpenAI-standard prompt_tokens_details.cached_tokens so the SDK
	// surfaces it as Usage.PromptTokensDetails.CachedTokens. Without this,
	// cache-hit tokens are billed at the full input price downstream.
	if needsUsageTap(cfg) {
		transport = newUsageTapTransport(transport)
		hasCustomTransport = true
	}
	if hasCustomTransport || cfg.RateLimitRPM > 0 || cfg.Retry.MaxAttempts != 0 || cfg.CB.Enabled {
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

// resolveTailoringStrategy maps a strategy string to the framework's
// TailoringStrategy implementation. Returns nil for empty/unrecognized
// values, letting the framework use its default (MiddleOut).
// Accepts both hyphen ("middle-out") and underscore ("middle_out") forms.
func resolveTailoringStrategy(s string) trpcmodel.TailoringStrategy {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-"))
	switch normalized {
	case "middle-out":
		return trpcmodel.NewMiddleOutStrategy(nil)
	case "head-out":
		return trpcmodel.NewHeadOutStrategy(nil)
	case "tail-out":
		return trpcmodel.NewTailOutStrategy(nil)
	default:
		return nil
	}
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
	if cfg.Cache.SystemPrompt {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPrompt(true))
	}
	if cfg.Cache.Tools {
		providerOpts = append(providerOpts, trpcanthropic.WithCacheTools(true))
	}
	if cfg.Cache.Messages {
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
	hasCustomTransport := rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil
	// 仅当有 API Key 或自定义 transport 时才构造 ClientConfig。
	// 拆分条件：避免仅有 transport 时传入空 API Key 导致 SDK 报错。
	if apiKey != "" || hasCustomTransport {
		// API-key auth must use GeminiAPI backend; VertexAI requires ADC.
		backend := genai.BackendGeminiAPI
		gcc := &genai.ClientConfig{
			Backend: backend,
		}
		// 仅在 API Key 非空时设置，避免传入空字符串。
		if apiKey != "" {
			gcc.APIKey = apiKey
		}
		if hasCustomTransport {
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
	switch strings.ToLower(strings.TrimSpace(cfg.HA.Mode)) {
	case "failover":
		return wrapFailover(cfg, rt, primary, lg)
	case "hedge":
		return wrapHedge(cfg, rt, primary, lg)
	}
	return primary, nil
}

func wrapFailover(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HA.Candidates {
		m, err := trpcModelFromCandidate(c, cfg, rt, lg)
		if err != nil {
			lg.Warn("HA failover 候选模型构建失败，跳过该候选",
				loggateway.StepID("provider.ha_failover_candidate_skip"),
				loggateway.Str("ha_mode", "failover"),
				loggateway.Str("candidate_name", c.Name),
				loggateway.Str("candidate_provider_type", c.ProviderType),
				loggateway.Err(err))
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
		trpcfailover.WithSwitchCallback(haSwitchCallback(lg, "failover", "system.provider.ha_failover")),
	)
	if err != nil {
		lg.Warn("HA failover 构建失败，回退到主模型", loggateway.StepID("provider.ha_failover_build_fail"), loggateway.Err(err))
		return primary, nil
	}
	return fo, nil
}

func wrapHedge(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
	candidates := []trpcmodel.Model{primary}
	for _, c := range cfg.HA.Candidates {
		m, err := trpcModelFromCandidate(c, cfg, rt, lg)
		if err != nil {
			lg.Warn("HA hedge 候选模型构建失败，跳过该候选",
				loggateway.StepID("provider.ha_hedge_candidate_skip"),
				loggateway.Str("ha_mode", "hedge"),
				loggateway.Str("candidate_name", c.Name),
				loggateway.Str("candidate_provider_type", c.ProviderType),
				loggateway.Err(err))
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
		trpchedge.WithSwitchCallback(haSwitchCallback(lg, "hedge", "system.provider.ha_hedge")),
	}
	if cfg.HA.HedgeDelayMs > 0 {
		hedgeOpts = append(hedgeOpts, trpchedge.WithDelay(time.Duration(cfg.HA.HedgeDelayMs)*time.Millisecond))
	}
	h, err := trpchedge.New(hedgeOpts...)
	if err != nil {
		lg.Warn("HA hedge 构建失败，回退到主模型", loggateway.StepID("provider.ha_hedge_build_fail"), loggateway.Err(err))
		return primary, nil
	}
	return h, nil
}

func haSwitchCallback(lg loggateway.Logger, mode, stepID string) func(fromIndex, toIndex int, fromName, toName, reason string) {
	return func(fromIndex, toIndex int, fromName, toName, reason string) {
		if lg == nil {
			return
		}
		lg.Warn("HA 候选模型切换",
			loggateway.StepID(stepID),
			loggateway.Str("ha_mode", mode),
			loggateway.Int("from_index", fromIndex),
			loggateway.Int("to_index", toIndex),
			loggateway.Str("from_model", fromName),
			loggateway.Str("to_model", toName),
			loggateway.Str("reason", reason),
		)
	}
}

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
	// Build a ProviderModelConfig from the candidate so it flows through the
	// same buildProviderOptions pipeline as the primary model (rate-limit,
	// variant, provider-specific options, etc.).
	// 继承主模型的可靠性配置（Retry/CB/RateLimitRPM），确保候选模型
	// 也有重试/熔断/限流保护，避免 HA 切换后候选模型因瞬时故障失败。
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
