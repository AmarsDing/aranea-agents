package plugintrpc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type auditConfig struct {
	LogModelRequest  bool `json:"log_model_request"`
	LogModelResponse bool `json:"log_model_response"`
	LogToolArgs      bool `json:"log_tool_args"`
	MaxContentLength int  `json:"max_content_length"`
	RedactSensitive  bool `json:"redact_sensitive"`
}

// AuditLogPlugin records agent/model/tool lifecycle events via the structured logger.
type AuditLogPlugin struct {
	name  string
	cfg   auditConfig
	stats StatsRecorder
}

var _ trpcplugin.Plugin = (*AuditLogPlugin)(nil)

// NewAuditLogPlugin builds the runtime audit plugin from a DB row (audit_log / runtime_audit).
func NewAuditLogPlugin(p biz.Plugin, stats StatsRecorder) *AuditLogPlugin {
	var cfg auditConfig
	cfg.LogModelRequest = true
	cfg.LogModelResponse = true
	cfg.LogToolArgs = true
	cfg.MaxContentLength = 500
	cfg.RedactSensitive = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	if cfg.MaxContentLength <= 0 {
		cfg.MaxContentLength = 500
	}
	return &AuditLogPlugin{name: p.Key, cfg: cfg, stats: stats}
}

func (a *AuditLogPlugin) Name() string { return a.name }

func (a *AuditLogPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeAgent(a.beforeAgent)
	r.AfterAgent(a.afterAgent)
	r.BeforeModel(a.beforeModel)
	r.AfterModel(a.afterModel)
	r.BeforeTool(a.beforeTool)
	r.AfterTool(a.afterTool)
	r.OnEvent(a.onEvent)
}

func (a *AuditLogPlugin) beforeAgent(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
	sid, akey := sessionAgentKey(ctx, args.Invocation)
	a.logLifecycle("before_agent", sid, akey)
	a.record(ctx, "before_agent", "ok")
	return &trpcagent.BeforeAgentResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) afterAgent(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
	status := "ok"
	if args != nil && args.Error != nil {
		status = "error"
	}
	sid, akey := sessionAgentKey(ctx, nil)
	if args != nil && args.Invocation != nil {
		sid, akey = sessionAgentKey(ctx, args.Invocation)
	}
	a.logLifecycle("after_agent", sid, akey, "status", status)
	a.record(ctx, "after_agent", status)
	return &trpcagent.AfterAgentResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	sid, akey := sessionAgentKey(ctx, nil)
	if a.cfg.LogModelRequest && args != nil && args.Request != nil {
		summary := a.summarizeMessages(args.Request)
		slog.Info("audit_log.model_request",
			"plugin", a.name,
			"session_id", sid,
			"agent_key", akey,
			"model", modelNameFromContext(ctx),
			"summary", summary,
		)
	} else {
		a.logLifecycle("before_model", sid, akey)
	}
	a.record(ctx, "before_model", "ok")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) afterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
	status := "ok"
	if args != nil && args.Error != nil {
		status = "error"
	}
	sid, akey := sessionAgentKey(ctx, nil)
	if a.cfg.LogModelResponse && args != nil && args.Response != nil {
		text := a.maybeRedact(responseText(args.Response))
		text = truncateString(text, a.cfg.MaxContentLength)
		slog.Info("audit_log.model_response",
			"plugin", a.name,
			"session_id", sid,
			"agent_key", akey,
			"status", status,
			"summary", text,
		)
	} else {
		a.logLifecycle("after_model", sid, akey, "status", status)
	}
	a.record(ctx, "after_model", status)
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args != nil && a.cfg.LogToolArgs {
		preview := truncateString(a.maybeRedact(string(args.Arguments)), a.cfg.MaxContentLength)
		sid, akey := sessionAgentKey(ctx, nil)
		slog.Info("audit_log.tool_call",
			"plugin", a.name,
			"tool", args.ToolName,
			"session_id", sid,
			"agent_key", akey,
			"args_preview", preview,
			"phase", "before",
		)
	}
	a.record(ctx, "before_tool", "ok")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil {
		a.record(ctx, "after_tool", "ok")
		return &trpctool.AfterToolResult{}, nil
	}
	status := "ok"
	if args.Error != nil {
		status = "error"
	}
	sid, akey := sessionAgentKey(ctx, nil)
	slog.Info("audit_log.tool_call",
		"plugin", a.name,
		"tool", args.ToolName,
		"session_id", sid,
		"agent_key", akey,
		"status", status,
		"phase", "after",
		"at", time.Now().UTC().Format(time.RFC3339),
	)
	a.record(ctx, "after_tool", status)
	return &trpctool.AfterToolResult{}, nil
}

func (a *AuditLogPlugin) onEvent(
	ctx context.Context,
	inv *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	if e == nil {
		return e, nil
	}
	sid, akey := sessionAgentKey(ctx, inv)
	kind := strings.TrimSpace(e.Object)
	if kind == "" {
		kind = "event"
	}
	slog.Info("audit_log.on_event",
		"plugin", a.name,
		"session_id", sid,
		"agent_key", akey,
		"event_object", kind,
		"author", e.Author,
	)
	a.record(ctx, "on_event", "ok")
	return e, nil
}

func (a *AuditLogPlugin) summarizeMessages(req *trpcmodel.Request) string {
	if req == nil {
		return ""
	}
	var parts []string
	for _, m := range req.Messages {
		line := strings.TrimSpace(string(m.Role) + ": " + m.Content)
		if line != "" {
			parts = append(parts, truncateString(a.maybeRedact(line), a.cfg.MaxContentLength))
		}
	}
	return strings.Join(parts, " | ")
}

func (a *AuditLogPlugin) maybeRedact(s string) string {
	if !a.cfg.RedactSensitive {
		return s
	}
	return redactText(s, true, true, true)
}

func (a *AuditLogPlugin) record(ctx context.Context, point, status string) {
	if a.stats != nil {
		a.stats.Record(ctx, a.name, point, status)
	}
}

func (a *AuditLogPlugin) logLifecycle(point, sessionID, agentKey string, extra ...any) {
	kv := []any{
		"plugin", a.name,
		"point", point,
		"session_id", sessionID,
		"agent_key", agentKey,
		"at", time.Now().UTC().Format(time.RFC3339),
	}
	kv = append(kv, extra...)
	slog.Info("audit_log.lifecycle", kv...)
}

func sessionAgentKey(ctx context.Context, inv *trpcagent.Invocation) (sessionID, agentKey string) {
	if inv != nil {
		agentKey = inv.AgentName
		if inv.Session != nil {
			sessionID = inv.Session.ID
		}
		return sessionID, agentKey
	}
	if i, ok := trpcagent.InvocationFromContext(ctx); ok && i != nil {
		agentKey = i.AgentName
		if i.Session != nil {
			sessionID = i.Session.ID
		}
	}
	return sessionID, agentKey
}
