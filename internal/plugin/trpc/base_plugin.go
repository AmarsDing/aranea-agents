package plugintrpc

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

type basePlugin struct {
	name   string
	stats  StatsRecorder
	logger *PluginSafeLogger
	lg     loggateway.Logger
}

func newBasePlugin(name string, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger) basePlugin {
	return basePlugin{
		name:   name,
		stats:  stats,
		logger: NewPluginSafeLogger(name, monitorBus, lg),
		lg:     lg,
	}
}

func (b *basePlugin) Name() string { return b.name }

func (b *basePlugin) record(_ context.Context, point, status string) {
	if b.stats != nil {
		b.stats.Record(context.Background(), b.name, point, status)
	}
}
