package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type auditConfig struct {
	LogModelRequest  bool `json:"log_model_request"`
	LogModelResponse bool `json:"log_model_response"`
	LogToolArgs      bool `json:"log_tool_args"`
	MaxContentLength int  `json:"max_content_length"`
	RedactSensitive  bool `json:"redact_sensitive"`
}

type AuditLogPlugin struct {
	base basePlugin
	cfg  auditConfig
}

var _ trpcplugin.Plugin = (*AuditLogPlugin)(nil)

func NewAuditLogPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger) *AuditLogPlugin {
	var cfg auditConfig
	cfg.LogModelRequest = true
	cfg.LogModelResponse = true
	cfg.LogToolArgs = true
	cfg.MaxContentLength = 500
	cfg.RedactSensitive = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	if cfg.MaxContentLength <= 0 {
		cfg.MaxContentLength = 500
	}
	return &AuditLogPlugin{
		base: newBasePlugin(p.Key, stats, monitorBus, lg),
		cfg:  cfg,
	}
}

func (a *AuditLogPlugin) Name() string { return a.base.Name() }

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
	a.base.record(ctx, "before_agent", "ok")
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
	a.base.record(ctx, "after_agent", status)
	return &trpcagent.AfterAgentResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	sid, akey := sessionAgentKey(ctx, nil)
	if a.cfg.LogModelRequest && args != nil && args.Request != nil {
		summary := a.summarizeMessages(args.Request)
		a.base.logger.Info("plugin.audit_log.before_model",
			"session_id", sid,
			"agent_key", akey,
			"model", modelNameFromContext(ctx),
			"summary", summary,
		)
	} else {
		a.logLifecycle("before_model", sid, akey)
	}
	a.base.record(ctx, "before_model", "ok")
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
		a.base.logger.Info("plugin.audit_log.after_model",
			"session_id", sid,
			"agent_key", akey,
			"status", status,
			"summary", text,
		)
	} else {
		a.logLifecycle("after_model", sid, akey, "status", status)
	}
	a.base.record(ctx, "after_model", status)
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args != nil && a.cfg.LogToolArgs {
		preview := truncateString(a.maybeRedact(string(args.Arguments)), a.cfg.MaxContentLength)
		sid, akey := sessionAgentKey(ctx, nil)
		a.base.logger.Info("plugin.audit_log.before_tool",
			"tool", args.ToolName,
			"session_id", sid,
			"agent_key", akey,
			"args_preview", preview,
		)
	}
	a.base.record(ctx, "before_tool", "ok")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (a *AuditLogPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil {
		a.base.record(ctx, "after_tool", "ok")
		return &trpctool.AfterToolResult{}, nil
	}
	status := "ok"
	if args.Error != nil {
		status = "error"
	}
	sid, akey := sessionAgentKey(ctx, nil)
	a.base.logger.Info("plugin.audit_log.after_tool",
		"tool", args.ToolName,
		"session_id", sid,
		"agent_key", akey,
		"status", status,
		"at", time.Now().UTC().Format(time.RFC3339),
	)
	a.base.record(ctx, "after_tool", status)
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
	a.base.logger.Info("plugin.audit_log.on_event",
		"session_id", sid,
		"agent_key", akey,
		"event_object", kind,
		"author", e.Author,
	)
	a.base.record(ctx, "on_event", "ok")
	return e, nil
}

func (a *AuditLogPlugin) maybeRedact(s string) string {
	if !a.cfg.RedactSensitive {
		return s
	}
	return redactText(s, true, true, true)
}

func (a *AuditLogPlugin) summarizeMessages(req *trpcmodel.Request) string {
	if req == nil {
		return ""
	}
	var b strings.Builder
	for i, msg := range req.Messages {
		if i > 0 {
			b.WriteString("; ")
		}
		role := string(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		content = a.maybeRedact(content)
		content = truncateString(content, a.cfg.MaxContentLength)
		fmt.Fprintf(&b, "[%s]%s", role, content)
	}
	return b.String()
}

func (a *AuditLogPlugin) logLifecycle(point, sessionID, agentKey string, extra ...any) {
	kv := []any{
		"point", point,
		"session_id", sessionID,
		"agent_key", agentKey,
	}
	kv = append(kv, extra...)
	a.base.logger.Info("plugin.audit_log."+point, kv...)
}
