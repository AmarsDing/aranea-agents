package plugintrpc

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

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
	name   string
	cfg    auditConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*AuditLogPlugin)(nil)

func NewAuditLogPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *AuditLogPlugin {
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
	name := p.Key
	return &AuditLogPlugin{
		name:   name,
		cfg:    cfg,
		stats:  stats,
		logger: NewPluginSafeLogger(name, bus),
	}
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
		fmt.Fprintf(os.Stderr, "[audit_log] model_request plugin=%s session_id=%s agent_key=%s model=%s summary=%s\n",
			a.name, sid, akey, modelNameFromContext(ctx), summary)
		os.Stderr.Sync()
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
		fmt.Fprintf(os.Stderr, "[audit_log] model_response plugin=%s session_id=%s agent_key=%s status=%s summary=%s\n",
			a.name, sid, akey, status, text)
		os.Stderr.Sync()
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
		fmt.Fprintf(os.Stderr, "[audit_log] tool_call plugin=%s tool=%s session_id=%s agent_key=%s args_preview=%s phase=before\n",
			a.name, args.ToolName, sid, akey, preview)
		os.Stderr.Sync()
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
	fmt.Fprintf(os.Stderr, "[audit_log] tool_call plugin=%s tool=%s session_id=%s agent_key=%s status=%s phase=after at=%s\n",
		a.name, args.ToolName, sid, akey, status, time.Now().UTC().Format(time.RFC3339))
	os.Stderr.Sync()
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
	fmt.Fprintf(os.Stderr, "[audit_log.on_event] plugin=%s session_id=%s agent_key=%s event_object=%s author=%s\n",
		a.name, sid, akey, kind, e.Author)
	os.Stderr.Sync()
	a.record(ctx, "on_event", "ok")
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
		content = truncateString(content, a.cfg.MaxContentLength)
		fmt.Fprintf(&b, "[%s]%s", role, content)
	}
	return b.String()
}

func (a *AuditLogPlugin) record(ctx context.Context, point, status string) {
	if a.stats != nil {
		a.stats.Record(ctx, a.name, point, status)
	}
}

func (a *AuditLogPlugin) logLifecycle(point, sessionID, agentKey string, extra ...any) {
	kv := []any{
		"point", point,
		"session_id", sessionID,
		"agent_key", agentKey,
	}
	kv = append(kv, extra...)
	a.logger.Info("plugin.audit_log."+point, kv...)
}
