package provider

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// omitThinkingKey reports whether outbound requests must drop thinking /
// reasoning fields. Catalog rows that explicitly mark capability_thinking=false
// (Ollama qwen2.5vl:7b and similar) reject "thinking":false with HTTP 400.
// Unknown / non-explicit models keep today's behaviour (DeepSeek still receives
// the disable key). Ollama without an explicit true capability never sends the
// key — omitting it is the provider default and avoids the 400.
func omitThinkingKey(pm biz.ProviderModel, cfg ProviderModelConfig) bool {
	if pm.CapabilitiesExplicit {
		return !pm.Capabilities.Thinking
	}
	pt := strings.ToLower(strings.TrimSpace(cfg.ProviderType))
	return pt == "ollama"
}

// ModelSupportsThinking reports whether the catalog model accepts a thinking
// toggle. Used at injection points (voice_fastpath / simple-turn disable) so
// we skip WithThinkingDisabled when the key would 400. Fail-open for unknown
// catalog rows except Ollama, matching omitThinkingKey.
func ModelSupportsThinking(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string, lg loggateway.Logger) bool {
	if looksLikeOllama(prov, model) && catalog == nil {
		return false
	}
	cfg, err := resolveCapabilities(ctx, catalog, prov, model)
	if err != nil {
		if lg != nil {
			lg.Warn("resolve capabilities failed, falling back to thinking heuristic",
				loggateway.StepID("provider.capability_fallback"),
				loggateway.Str("provider", prov),
				loggateway.Str("model", model),
				loggateway.Err(err))
		}
		return !looksLikeOllama(prov, model)
	}
	if cfg == nil {
		return !looksLikeOllama(prov, model)
	}
	if hasExplicitCapabilities(cfg.Capabilities) {
		return cfg.Capabilities.Thinking
	}
	pt := strings.ToLower(strings.TrimSpace(cfg.ProviderType))
	if pt == "ollama" {
		return false
	}
	return !looksLikeOllama(prov, model)
}

func looksLikeOllama(parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(strings.TrimSpace(part)), "ollama") {
			return true
		}
	}
	return false
}

// wrapOmitThinking strips thinking/reasoning fields before the inner model
// serializes the request. Defense in depth for every injection site
// (voice_fastpath, generationConfigForAgent, intent, llmcompat).
func wrapOmitThinking(inner trpcmodel.Model) trpcmodel.Model {
	if inner == nil {
		return nil
	}
	base := &omitThinkingModel{inner: inner}
	if _, ok := inner.(trpcmodel.IterModel); ok {
		return &omitThinkingIterModel{omitThinkingModel: base}
	}
	return base
}

type omitThinkingModel struct {
	inner trpcmodel.Model
}

func stripThinkingFields(req *trpcmodel.Request) {
	if req == nil {
		return
	}
	req.GenerationConfig.ThinkingEnabled = nil
	req.GenerationConfig.ReasoningEffort = nil
	req.GenerationConfig.ThinkingTokens = nil
}

func (m *omitThinkingModel) Info() trpcmodel.Info {
	return m.inner.Info()
}

func (m *omitThinkingModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	stripThinkingFields(request)
	return m.inner.GenerateContent(ctx, request)
}

type omitThinkingIterModel struct {
	*omitThinkingModel
}

func (m *omitThinkingIterModel) GenerateContentIter(ctx context.Context, request *trpcmodel.Request) (trpcmodel.Seq[*trpcmodel.Response], error) {
	iter, ok := m.inner.(trpcmodel.IterModel)
	if !ok {
		return nil, ErrModelNilChannel
	}
	stripThinkingFields(request)
	return iter.GenerateContentIter(ctx, request)
}
