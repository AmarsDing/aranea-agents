package plugintrpc

import (
	"context"
	"time"

	"aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// productEventPlugin wires Runner OnEvent to hook rules (on_event) and metrics.
type productEventPlugin struct {
	mgr  *Manager
	name string
}

var _ trpcplugin.Plugin = (*productEventPlugin)(nil)

func (p *productEventPlugin) Name() string { return p.name }

func (p *productEventPlugin) Register(r *trpcplugin.Registry) {
	if r == nil || p == nil || p.mgr == nil {
		return
	}
	r.OnEvent(p.onEvent)
}

func (p *productEventPlugin) onEvent(
	ctx context.Context,
	invocation *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	start := time.Now()
	out, err := p.mgr.dispatchHookOnEvent(ctx, invocation, e)
	metrics.ObserveCallback("hook", "on_event", start, err)
	if err != nil {
		metrics.PluginInvokeTotal.WithLabelValues("event_bridge", "on_event", "error").Inc()
		return out, err
	}
	metrics.PluginInvokeTotal.WithLabelValues("event_bridge", "on_event", "ok").Inc()
	return out, nil
}
