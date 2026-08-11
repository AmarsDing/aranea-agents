package plugintrpc

import (
	"context"
	"strings"

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

// recordSummaryMaxLen 与 audit_log max_content_length 默认口径一致。
const recordSummaryMaxLen = 500

// recordEvent 在 record 之上携带诊断信息（session/agent/summary），
// 供 blocked/error 等会持久化到 plugin_runs 的路径使用，便于排障。
func (b *basePlugin) recordEvent(ctx context.Context, point, status, summary string) {
	sid, akey := sessionAgentKey(ctx, nil)
	b.recordEventAt(sid, akey, point, status, summary)
}

// recordEventAt 显式传入 session/agent 的 recordEvent 版本，
// 供异步记录（原 request ctx 已失效）的调用点使用。
func (b *basePlugin) recordEventAt(sessionID, agentKey, point, status, summary string) {
	if b.stats == nil {
		return
	}
	summary = strings.TrimSpace(summary)
	summary = truncateString(summary, recordSummaryMaxLen)
	b.stats.RecordEvent(context.Background(), CallbackEvent{
		PluginKey: b.name,
		Point:     strings.TrimSpace(point),
		Status:    strings.TrimSpace(status),
		Action:    callbackAction(status),
		SessionID: strings.TrimSpace(sessionID),
		AgentKey:  strings.TrimSpace(agentKey),
		Summary:   summary,
	})
}
