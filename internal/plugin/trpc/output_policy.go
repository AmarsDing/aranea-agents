package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type outputPolicyConfig struct {
	BlockedPatterns       []string `json:"blocked_patterns"`
	DangerousCommandCheck bool     `json:"dangerous_command_check"`
	BlockOnViolation      bool     `json:"block_on_violation"`
	ReplacementMessage    string   `json:"replacement_message"`
}

type OutputPolicyPlugin struct {
	name   string
	cfg    outputPolicyConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*OutputPolicyPlugin)(nil)

func NewOutputPolicyPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *OutputPolicyPlugin {
	var cfg outputPolicyConfig
	cfg.DangerousCommandCheck = true
	cfg.BlockOnViolation = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	name := p.Key
	return &OutputPolicyPlugin{
		name:   name,
		cfg:    cfg,
		stats:  stats,
		logger: NewPluginSafeLogger(name, bus),
	}
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
		o.logger.Info("plugin.output_policy.after_model", "status", "blocked", "pattern", pat, "block_on_violation", o.cfg.BlockOnViolation)
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
	} else {
		o.logger.Info("plugin.output_policy.after_model", "status", "ok")
	}
	o.record(ctx, "after_model", "ok")
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (o *OutputPolicyPlugin) onEvent(
	ctx context.Context,
	inv *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	if e == nil || e.Response == nil {
		return e, nil
	}
	text := eventText(e)
	if text == "" {
		return e, nil
	}
	viol, pat := o.violation(text)
	if !viol {
		return e, nil
	}
	o.record(ctx, "on_event", "blocked")
	o.logger.Warn("output_policy.event_blocked", "plugin", o.name, "pattern", pat, "block_on_violation", o.cfg.BlockOnViolation)
	// TPM-P1-04: actually enforce block_on_violation in streaming path. Previously
	// the event passed through unchanged — admin's block_on_violation=true was a no-op
	// for OnEvent (only afterModel honored it). Now we splice the chunk content with
	// the replacement message so the violating fragment never reaches the client.
	if !o.cfg.BlockOnViolation {
		return e, nil
	}
	msg := strings.TrimSpace(o.cfg.ReplacementMessage)
	if msg == "" {
		msg = "output_policy: blocked content matching " + pat
	}
	for i := range e.Response.Choices {
		ch := &e.Response.Choices[i]
		ch.Message.Content = msg
		ch.Delta.Content = msg
		if ch.FinishReason == nil {
			reason := "content_filter"
			ch.FinishReason = &reason
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
