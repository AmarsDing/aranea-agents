package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type modelRouterConfig struct {
	DefaultModel      string `json:"default_model"`
	CodeModel         string `json:"code_model"`
	LongContextModel  string `json:"long_context_model"`
	FallbackModel     string `json:"fallback_model"`
}

type ModelRouterPlugin struct {
	name  string
	cfg   modelRouterConfig
	stats StatsRecorder
}

var _ trpcplugin.Plugin = (*ModelRouterPlugin)(nil)

func NewModelRouterPlugin(p biz.Plugin, stats StatsRecorder) *ModelRouterPlugin {
	var cfg modelRouterConfig
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &ModelRouterPlugin{name: p.Key, cfg: cfg, stats: stats}
}

func (m *ModelRouterPlugin) Name() string { return m.name }

func (m *ModelRouterPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(m.beforeModel)
}

func (m *ModelRouterPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	if args == nil || args.Request == nil {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	}
	prompt := strings.ToLower(promptText(args.Request))
	target := strings.TrimSpace(m.cfg.DefaultModel)
	switch {
	case m.cfg.CodeModel != "" && looksLikeCodeTask(prompt):
		target = strings.TrimSpace(m.cfg.CodeModel)
	case m.cfg.LongContextModel != "" && len(prompt) > 12000:
		target = strings.TrimSpace(m.cfg.LongContextModel)
	}
	if target != "" {
		patchRequestModelHint(args.Request, target)
	} else if fb := strings.TrimSpace(m.cfg.FallbackModel); fb != "" && modelNameFromContext(ctx) == "" {
		patchRequestModelHint(args.Request, fb)
	}
	m.record(ctx, "before_model", "ok")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
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
