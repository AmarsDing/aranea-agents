package plugintrpc

import (
	"context"
	"log/slog"
	"strings"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type outputPolicyConfig struct {
	BlockedPatterns         []string `json:"blocked_patterns"`
	DangerousCommandCheck   bool     `json:"dangerous_command_check"`
	BlockOnViolation        bool     `json:"block_on_violation"`
	ReplacementMessage      string   `json:"replacement_message"`
}

type OutputPolicyPlugin struct {
	name  string
	cfg   outputPolicyConfig
	stats StatsRecorder
}

var _ trpcplugin.Plugin = (*OutputPolicyPlugin)(nil)

func NewOutputPolicyPlugin(p biz.Plugin, stats StatsRecorder) *OutputPolicyPlugin {
	var cfg outputPolicyConfig
	cfg.DangerousCommandCheck = true
	cfg.BlockOnViolation = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &OutputPolicyPlugin{name: p.Key, cfg: cfg, stats: stats}
}

func (o *OutputPolicyPlugin) Name() string { return o.name }

func (o *OutputPolicyPlugin) Register(r *trpcplugin.Registry) {
	r.AfterModel(o.afterModel)
	r.OnEvent(o.onEvent)
}

func (o *OutputPolicyPlugin) afterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
	if args == nil || args.Response == nil {
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	}
	text := responseText(args.Response)
	if viol, pat := o.violation(text); viol {
		o.record(ctx, "after_model", "blocked")
		if o.cfg.BlockOnViolation {
			msg := strings.TrimSpace(o.cfg.ReplacementMessage)
			if msg == "" {
				msg = "output_policy: blocked content matching " + pat
			}
			return &trpcmodel.AfterModelResult{
				Context:        ctx,
				CustomResponse: blockedModelResponse(msg),
			}, nil
		}
		slog.Warn("output_policy.violation", "plugin", o.name, "pattern", pat)
	}
	o.record(ctx, "after_model", "ok")
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (o *OutputPolicyPlugin) onEvent(
	ctx context.Context,
	inv *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	if e == nil {
		return e, nil
	}
	text := eventText(e)
	if text == "" {
		return e, nil
	}
	if viol, pat := o.violation(text); viol {
		o.record(ctx, "on_event", "blocked")
		if o.cfg.BlockOnViolation {
			slog.Warn("output_policy.event_blocked", "plugin", o.name, "pattern", pat)
		}
	}
	return e, nil
}

func (o *OutputPolicyPlugin) violation(text string) (bool, string) {
	if containsAny(text, o.cfg.BlockedPatterns) {
		for _, p := range o.cfg.BlockedPatterns {
			if strings.TrimSpace(p) != "" && strings.Contains(strings.ToLower(text), strings.ToLower(p)) {
				return true, p
			}
		}
		return true, "blocked_patterns"
	}
	if o.cfg.DangerousCommandCheck {
		danger := []string{"rm -rf", "drop table", "format c:"}
		if containsAny(text, danger) {
			return true, "dangerous_command"
		}
	}
	return false, ""
}

func eventText(e *trpcevent.Event) string {
	if e == nil || e.Response == nil {
		return ""
	}
	return responseText(e.Response)
}

func (o *OutputPolicyPlugin) record(ctx context.Context, point, status string) {
	if o.stats != nil {
		o.stats.Record(ctx, o.name, point, status)
	}
}
