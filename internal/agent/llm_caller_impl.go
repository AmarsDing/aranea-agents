package agent

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// OpenAICompatLLMCaller implements biz.LLMCaller using CallOpenAICompatChat
// with a statically-configured ProviderAPIConfig. Mainly for tests and
// fixed-credential scenarios; production code uses DynamicLLMCaller.
// PGO-3-BIZ-01.
type OpenAICompatLLMCaller struct {
	cfg ProviderAPIConfig
	hc  *http.Client
}

// NewOpenAICompatLLMCaller creates a caller from a biz.LLMCallerConfig.
func NewOpenAICompatLLMCaller(cfg biz.LLMCallerConfig) *OpenAICompatLLMCaller {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAICompatLLMCaller{
		cfg: ProviderAPIConfig{
			ProviderType: cfg.Provider,
			APIBaseURL:   cfg.BaseURL,
			APIKey:       cfg.APIKey,
		},
		hc: &http.Client{Timeout: timeout},
	}
}

// NewOpenAICompatLLMCallerFromProviderConfig creates a caller directly from ProviderAPIConfig.
func NewOpenAICompatLLMCallerFromProviderConfig(cfg ProviderAPIConfig, timeoutSec int) *OpenAICompatLLMCaller {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAICompatLLMCaller{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

// Call implements biz.LLMCaller. If req.Provider is set it overrides cfg.ProviderType
// so PromptRefiner's decision wins (B-1 fix). The model name is taken from req.Model
// (B-4 fix) — previously empty string was passed.
func (c *OpenAICompatLLMCaller) Call(ctx context.Context, req biz.LLMCallRequest) (string, int, error) {
	msgs := buildMessages(req.System, req.User, req.Images...)
	cfg := c.cfg
	if p := strings.TrimSpace(req.Provider); p != "" {
		cfg.ProviderType = p
	}
	modelName := strings.TrimSpace(req.Model)
	text, _, promptTok, completionTok, err := CallOpenAICompatChat(ctx, c.hc, cfg, modelName, msgs)
	if err != nil {
		return "", 0, err
	}
	return text, promptTok + completionTok, nil
}

var _ biz.LLMCaller = (*OpenAICompatLLMCaller)(nil)

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
func (c *DynamicLLMCaller) Call(ctx context.Context, req biz.LLMCallRequest) (string, int, error) {
	cfg, err := c.resolveCredentials(ctx, req.Provider, req.Model)
	if err != nil {
		return "", 0, err
	}
	msgs := buildMessages(req.System, req.User, req.Images...)
	modelName := strings.TrimSpace(req.Model)
	text, _, promptTok, completionTok, err := CallOpenAICompatChat(ctx, c.hc, cfg, modelName, msgs)
	if err != nil {
		return "", 0, err
	}
	return text, promptTok + completionTok, nil
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
		row, err := c.catalog.GetByProviderAndModel(ctx, m.Provider, m.Model)
		if err != nil {
			return ProviderAPIConfig{}, false
		}
		var cfg ProviderAPIConfig
		MergeProviderConfigJSON(row.ConfigJSON, &cfg)
		cfg.ProviderType = row.Provider
		if strings.TrimSpace(cfg.APIBaseURL) == "" {
			return ProviderAPIConfig{}, false
		}
		return cfg, true
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
