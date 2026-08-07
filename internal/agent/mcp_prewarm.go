package agent

import (
	"context"

	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

// PrewarmMCPToolSets establishes pooled MCP connections for all enabled
// platform MCP servers at startup, so the first user request does not pay the
// cold-connect cost (stdio process spawn / TCP handshake + initialize +
// tools/list). Connections enter the process-level pool and stay warm until
// the idle reaper collects them.
//
// Servers requiring per-user credentials are skipped: they bypass the pool by
// design (credential isolation) and per-user headers are unavailable at
// startup. Individual server failures are logged and skipped — pre-warm is a
// latency optimization, never a startup blocker (Always-Ready).
func PrewarmMCPToolSets(ctx context.Context, deps TRPCBuilderDeps, lg loggateway.Logger) {
	if deps.MCPTooling == nil {
		return
	}
	servers, err := deps.MCPTooling.AllEnabledServers(ctx)
	if err != nil {
		lg.Warn("MCP 预热：服务器列表获取失败",
			loggateway.StepID("agent.mcp_prewarm"),
			loggateway.Err(err))
		return
	}
	if len(servers) == 0 {
		return
	}
	platformAllowAdHoc := platformMCPAllowAdHocHTTP(ctx, deps)
	warmed, skipped := 0, 0
	for _, s := range servers {
		cfg, ok := mcpToolServerConfig(ctx, deps, s, platformAllowAdHoc, lg)
		if !ok {
			skipped++
			continue
		}
		if cfg.RequireUserCredentials {
			skipped++
			continue
		}
		if err := tools.PrewarmMCPToolSet(ctx, cfg); err != nil {
			lg.Warn("MCP 预热：连接建立失败（降级，首次调用时重试）",
				loggateway.StepID("agent.mcp_prewarm"),
				loggateway.Str("server", cfg.Name),
				loggateway.Err(err))
			skipped++
			continue
		}
		warmed++
	}
	lg.Info("MCP 连接预热完成",
		loggateway.StepID("agent.mcp_prewarm"),
		loggateway.Int("warmed", warmed),
		loggateway.Int("skipped", skipped),
		loggateway.Int("total", len(servers)))
}
