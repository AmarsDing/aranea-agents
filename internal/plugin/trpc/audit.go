package plugintrpc

import (
	"context"
	"log/slog"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// AuditLogPlugin records every tool call via the structured logger.
// In S5 this will be replaced by a write to an audit_log DB table.
type AuditLogPlugin struct {
	name string
}

var _ trpcplugin.Plugin = (*AuditLogPlugin)(nil)

func (a *AuditLogPlugin) Name() string { return a.name }

func (a *AuditLogPlugin) Register(r *trpcplugin.Registry) {
	r.AfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		status := "ok"
		if args.Error != nil {
			status = "error"
		}
		var sessionID, agentKey string
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			agentKey = inv.AgentName
			if inv.Session != nil {
				sessionID = inv.Session.ID
			}
		}
		slog.Info("audit_log.tool_call",
			"plugin", a.name,
			"tool", args.ToolName,
			"session_id", sessionID,
			"agent_key", agentKey,
			"status", status,
			"at", time.Now().UTC().Format(time.RFC3339),
		)
		return &trpctool.AfterToolResult{}, nil
	})
}
