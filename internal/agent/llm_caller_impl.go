package agent

import (
	"context"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ─── DynamicLLMCaller ────────────────────────────────────────────────────────

// LLMCredentialResolver resolves provider credentials for LLM calls.
type LLMCredentialResolver interface {
	List(ctx context.Context) ([]biz.ProviderModel, error)
	// GetByProviderAndModel returns the decrypted runtime config for one
	// catalog row. Required because List is sanitized for API display
	// (api_key stripped) and must not be used for outbound credentials.
	GetByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error)
}

// LLMRefineConfigResolver resolves the default refine LLM config.
type LLMRefineConfigResolver interface {
	GetRefineLLM(ctx context.Context) (biz.RefineLLMSetting, error)
}

// DynamicLLMCaller implements biz.LLMCaller by resolving BaseURL + APIKey at
// call time, given the (Provider, Model) decision made by PromptRefiner.
// Credentials come from SystemSetting.DefaultRefineLLM or the model catalog.
//
// PGO-3-BIZ-01 (B-1/B-4 fix): previously this caller re-resolved provider/model
// itself, making the response's Provider/Model fields lie to the user.
// Now the caller honors the request's (Provider, Model) and only resolves
// credentials.
type DynamicLLMCaller struct {
	catalog LLMCredentialResolver
	sys     LLMRefineConfigResolver
	hc      *http.Client
	// callFn 是可注入的底层调用（测试替身）；nil 时使用 CallOpenAICompatChat。
	callFn func(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage) (string, string, int, int, error)
}

// NewDynamicLLMCaller wires a DynamicLLMCaller for use in Wire.
// The RoundTrip provides the centralized HTTP client; timeout is configured
// by the Wire provider rather than created ad-hoc here.
func NewDynamicLLMCaller(sys LLMRefineConfigResolver, catalog LLMCredentialResolver, rt *provider.RoundTrip) *DynamicLLMCaller {
	return &DynamicLLMCaller{
		sys:     sys,
		catalog: catalog,
		hc:      rt.Client(),
	}
}

// Call honors req.Provider + req.Model and resolves matching credentials.
//
// P2-2 fallback：主模型调用失败（网络/5xx/鉴权等任一侧信道错误）时，在目录内
// 选同 provider 的下一个 enabled 模型降级重试一次——PromptRefiner / 意图识别等
// 旁路调用没有 HA 包装，单次故障即整轮失败；候选自身失败不再递归（避免隐性
// 级联放大延迟与成本）。调用方 ctx 已取消/超时不降级（用户已放弃）。
func (c *DynamicLLMCaller) Call(ctx context.Context, req biz.LLMCallRequest) (string, int, error) {
	fn := c.callFn
	if fn == nil {
		fn = CallOpenAICompatChat
	}
	cfg, err := c.resolveCredentials(ctx, req.Provider, req.Model)
	if err != nil {
		return "", 0, err
	}
	// P2-5：透传思考强度路由决策（按次调用维度，主模型与降级候选同档——
	// effort 属于任务而非模型）。
	cfg.ThinkingEffort = req.ThinkingEffort
	msgs := buildMessages(req.System, req.User, req.Images...)
	modelName := strings.TrimSpace(req.Model)
	text, _, promptTok, completionTok, err := fn(ctx, c.hc, cfg, modelName, msgs)
	if err == nil {
		return text, promptTok + completionTok, nil
	}
	if ctx.Err() != nil {
		return "", 0, err
	}
	fb, ok := c.fallbackCandidate(ctx, req.Provider, modelName)
	if !ok {
		return "", 0, err
	}
	fbCfg, cfgOK := c.decryptedCatalogConfig(ctx, fb.Provider, fb.Model)
	if !cfgOK {
		return "", 0, err
	}
	fbCfg.ThinkingEffort = req.ThinkingEffort
	// K3 降级：流程日志（ctx 无 TraceEmitter 时静默跳过，同 cascade 约定）。
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogWarn("chat.llm.fallback", "模型降级重试", err.Error(),
			event.P("provider", strings.TrimSpace(req.Provider)),
			event.P("primary_model", modelName),
			event.P("fallback_model", fb.Model))
	}
	text, _, promptTok, completionTok, fbErr := fn(ctx, c.hc, fbCfg, fb.Model, msgs)
	if fbErr != nil {
		return "", 0, fbErr
	}
	return text, promptTok + completionTok, nil
}

// fallbackCandidate 选择降级候选：同 provider、enabled、非主模型，取目录序首个。
func (c *DynamicLLMCaller) fallbackCandidate(ctx context.Context, providerName, primaryModel string) (biz.ProviderModel, bool) {
	providerName = strings.TrimSpace(providerName)
	if c.catalog == nil || providerName == "" {
		return biz.ProviderModel{}, false
	}
	models, err := c.catalog.List(ctx)
	if err != nil {
		return biz.ProviderModel{}, false
	}
	for i := range models {
		m := models[i]
		if !m.Enabled || m.Provider != providerName {
			continue
		}
		if primaryModel != "" && m.Model == primaryModel {
			continue
		}
		return m, true
	}
	return biz.ProviderModel{}, false
}

// decryptedCatalogConfig 重新按行拉取解密后的运行时配置（List 行已脱敏，
// api_key 被剥离——见 sanitizeProviderModelForAPI）。
func (c *DynamicLLMCaller) decryptedCatalogConfig(ctx context.Context, providerName, modelName string) (ProviderAPIConfig, bool) {
	row, err := c.catalog.GetByProviderAndModel(ctx, providerName, modelName)
	if err != nil {
		return ProviderAPIConfig{}, false
	}
	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	cfg.ProviderType = row.Provider
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)
	if strings.TrimSpace(cfg.APIBaseURL) == "" {
		return ProviderAPIConfig{}, false
	}
	return cfg, true
}

// resolveCredentials returns a ProviderAPIConfig with BaseURL + APIKey for
// the (provider, model) pair. Lookup order:
//  1. Exact catalog row matching provider+model → its decrypted ConfigJSON
//  2. SystemSetting.DefaultRefineLLM if its Provider matches → use its BaseURL/APIKey
//  3. SystemSetting.DefaultRefineLLM regardless (best-effort)
//  4. First enabled catalog row with the same Provider → its decrypted ConfigJSON
//  5. Error.
//
// Note: the catalog List is sanitized for API display (api_key stripped), so
// matching rows must be re-fetched via GetByProviderAndModel to obtain the
// decrypted runtime credentials.
func (c *DynamicLLMCaller) resolveCredentials(ctx context.Context, provider, model string) (ProviderAPIConfig, error) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)

	// Strategy 1 + 4: walk the catalog to locate matching enabled rows.
	var providerMatch *biz.ProviderModel
	var exactMatch *biz.ProviderModel
	if c.catalog != nil {
		models, err := c.catalog.List(ctx)
		if err == nil {
			for i := range models {
				m := &models[i]
				if !m.Enabled || m.Provider == "" {
					continue
				}
				if m.Provider == provider && m.Model == model && model != "" {
					exactMatch = m
				}
				if providerMatch == nil && m.Provider == provider {
					providerMatch = m
				}
			}
		}
	}
	// decryptedConfig re-fetches the row to obtain the decrypted runtime
	// config (List rows carry no api_key — see sanitizeProviderModelForAPI).
	decryptedConfig := func(m *biz.ProviderModel) (ProviderAPIConfig, bool) {
		return c.decryptedCatalogConfig(ctx, m.Provider, m.Model)
	}

	// Strategy 1: exact provider+model match.
	if exactMatch != nil {
		if cfg, ok := decryptedConfig(exactMatch); ok {
			return cfg, nil
		}
	}

	// Strategy 2 + 3: SystemSetting fallback.
	// Note: API key is sensitive — read it via the dedicated GetRefineLLM
	// usecase method, not via Get() which deliberately redacts the key.
	if c.sys != nil {
		rl, err := c.sys.GetRefineLLM(ctx)
		if err == nil && strings.TrimSpace(rl.BaseURL) != "" {
			// Strategy 2: SystemSetting Provider matches the requested provider.
			if strings.TrimSpace(rl.Provider) == provider {
				cfg := ProviderAPIConfig{
					ProviderType: provider,
					APIBaseURL:   rl.BaseURL,
					APIKey:       rl.APIKey,
				}
				return cfg, nil
			}
			// Strategy 3: Provider mismatch — use as last-resort fallback,
			// but warn because the BaseURL may be wrong for this provider.
			if provider != "" && strings.TrimSpace(rl.Provider) != "" && strings.TrimSpace(rl.Provider) != provider {
				// Log a warning but still allow the fallback for backward compatibility.
				// The caller should check if the resulting config makes sense.
			}
			cfg := ProviderAPIConfig{
				ProviderType: provider,
				APIBaseURL:   rl.BaseURL,
				APIKey:       rl.APIKey,
			}
			if cfg.ProviderType == "" {
				cfg.ProviderType = rl.Provider
			}
			return cfg, nil
		}
	}

	// Strategy 4: same-provider catalog fallback.
	if providerMatch != nil {
		if cfg, ok := decryptedConfig(providerMatch); ok {
			return cfg, nil
		}
	}

	return ProviderAPIConfig{}, biz.ErrRefineNoLLMAvailable()
}

var _ biz.LLMCaller = (*DynamicLLMCaller)(nil)

// buildMessages converts (system, user) strings into the OpenAI-compat message array.
// images 非空时 user 消息携带多模态 ContentParts（文本 + 图片）。
func buildMessages(system, user string, images ...biz.LLMImage) []OpenAICompatMessage {
	msgs := make([]OpenAICompatMessage, 0, 2)
	if s := strings.TrimSpace(system); s != "" {
		msgs = append(msgs, OpenAICompatMessage{Role: "system", Content: s})
	}
	if len(images) > 0 {
		parts := make([]trpcmodel.ContentPart, 0, len(images)+1)
		if u := strings.TrimSpace(user); u != "" {
			parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &u})
		}
		for _, img := range images {
			if len(img.Data) == 0 {
				continue
			}
			parts = append(parts, trpcmodel.ContentPart{
				Type:  trpcmodel.ContentTypeImage,
				Image: &trpcmodel.Image{Data: img.Data, Format: img.Format},
			})
		}
		msgs = append(msgs, OpenAICompatMessage{Role: "user", ContentParts: parts})
		return msgs
	}
	if u := strings.TrimSpace(user); u != "" {
		msgs = append(msgs, OpenAICompatMessage{Role: "user", Content: u})
	}
	return msgs
}
