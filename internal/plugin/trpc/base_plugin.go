package plugintrpc

import (
	"context"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

type basePlugin struct {
	name   string
	stats  StatsRecorder
	logger *PluginSafeLogger
	lg     loggateway.Logger
}

func newBasePlugin(name string, stats StatsRecorder, bus event.Bus, lg loggateway.Logger) basePlugin {
	return basePlugin{
		name:   name,
		stats:  stats,
		logger: NewPluginSafeLogger(name, bus, lg),
		lg:     lg,
	}
}

func (b *basePlugin) Name() string { return b.name }

func (b *basePlugin) record(_ context.Context, point, status string) {
	if b.stats != nil {
		b.stats.Record(context.Background(), b.name, point, status)
	}
}
