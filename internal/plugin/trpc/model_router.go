package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// ModelRouterConfig is the product configuration for model_router plugin routing.
type ModelRouterConfig struct {
	Rules            []ModelRouterRule `json:"rules"`
	DefaultModel     string            `json:"default_model"`
	CodeModel        string            `json:"code_model"`
	LongContextModel string            `json:"long_context_model"`
	FallbackModel    string            `json:"fallback_model"`
}

var codeTaskMarkers = []string{"```", "function ", "def ", "class ", "import ", "bug", "refactor", "compile"}

const longContextCharThreshold = 12000

type ModelRouterPlugin struct {
	base basePlugin
	cfg  ModelRouterConfig
}

var _ trpcplugin.Plugin = (*ModelRouterPlugin)(nil)

func NewModelRouterPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *ModelRouterPlugin {
	var cfg ModelRouterConfig
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	compileModelRouterRules(cfg.Rules)
	return &ModelRouterPlugin{base: newBasePlugin(p.Key, stats, bus), cfg: cfg}
}

func (m *ModelRouterPlugin) Name() string { return m.base.Name() }

func (m *ModelRouterPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(m.beforeModel)
}

// beforeModel records telemetry only. Catalog routing is handled by agent.PluginModelSelector.
func (m *ModelRouterPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	origModel := modelNameFromContext(ctx)
	target := ""
	if args != nil && args.Request != nil {
		target = ResolveModelAPI(promptText(args.Request), m.cfg)
	}
	if target != "" && target != origModel {
		m.base.logger.Info("plugin.model_router.before_model", "status", "routed_via_selector", "orig_model", origModel, "target_model", target)
	} else {
		m.base.logger.Info("plugin.model_router.before_model", "status", "no_route", "orig_model", origModel)
	}
	m.base.record(ctx, "before_model", "ok")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

// ResolveModelAPI picks a catalog model API id from prompt heuristics and plugin config.
func ResolveModelAPI(prompt string, cfg ModelRouterConfig) string {
	if target := resolveModelFromRules(prompt, cfg.Rules); target != "" {
		return target
	}
	promptLower := strings.ToLower(prompt)
	target := strings.TrimSpace(cfg.DefaultModel)
	switch {
	case cfg.CodeModel != "" && looksLikeCodeTask(promptLower):
		target = strings.TrimSpace(cfg.CodeModel)
	case cfg.LongContextModel != "" && len(prompt) > longContextCharThreshold:
		target = strings.TrimSpace(cfg.LongContextModel)
	}
	if target != "" {
		return target
	}
	return strings.TrimSpace(cfg.FallbackModel)
}

func promptText(req *trpcmodel.Request) string {
	if req == nil {
		return ""
	}
	var b strings.Builder
	for _, msg := range req.Messages {
		b.WriteString(msg.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func looksLikeCodeTask(prompt string) bool {
	for _, m := range codeTaskMarkers {
		if strings.Contains(prompt, m) {
			return true
		}
	}
	return false
}

