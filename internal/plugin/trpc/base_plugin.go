package plugintrpc

import (
	"context"

	"aranea-agents/internal/event"
)

type basePlugin struct {
	name   string
	stats  StatsRecorder
	logger *PluginSafeLogger
}

func newBasePlugin(name string, stats StatsRecorder, bus event.Bus) basePlugin {
	return basePlugin{
		name:   name,
		stats:  stats,
		logger: NewPluginSafeLogger(name, bus),
	}
}

func (b *basePlugin) Name() string { return b.name }

func (b *basePlugin) record(_ context.Context, point, status string) {
	if b.stats != nil {
		b.stats.Record(context.Background(), b.name, point, status)
	}
}
