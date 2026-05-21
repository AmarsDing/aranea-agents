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

type modelRouterConfig = ModelRouterConfig

type ModelRouterPlugin struct {
	name   string
	cfg    modelRouterConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*ModelRouterPlugin)(nil)

func NewModelRouterPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *ModelRouterPlugin {
	var cfg modelRouterConfig
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &ModelRouterPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus)}
}

func (m *ModelRouterPlugin) Name() string { return m.name }

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
		m.logger.Info("plugin.model_router.before_model", "status", "routed_via_selector", "orig_model", origModel, "target_model", target)
	} else {
		m.logger.Info("plugin.model_router.before_model", "status", "no_route", "orig_model", origModel)
	}
	m.record(ctx, "before_model", "ok")
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
	case cfg.LongContextModel != "" && len(prompt) > 12000:
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
	markers := []string{"```", "function ", "def ", "class ", "import ", "bug", "refactor", "compile"}
	for _, m := range markers {
		if strings.Contains(prompt, m) {
			return true
		}
	}
	return false
}

func (m *ModelRouterPlugin) record(ctx context.Context, point, status string) {
	if m.stats != nil {
		m.stats.Record(ctx, m.name, point, status)
	}
}
