package plugintrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type confirmationGuardConfig struct {
	ConfirmTools    []string `json:"confirm_tools"`
	ConfirmPatterns []string `json:"confirm_patterns"`
	DefaultAction   string   `json:"default_action"`
}

type ConfirmationGuardPlugin struct {
	name  string
	cfg   confirmationGuardConfig
	stats StatsRecorder
}

var _ trpcplugin.Plugin = (*ConfirmationGuardPlugin)(nil)

func NewConfirmationGuardPlugin(p biz.Plugin, stats StatsRecorder) *ConfirmationGuardPlugin {
	var cfg confirmationGuardConfig
	cfg.DefaultAction = "reject"
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &ConfirmationGuardPlugin{name: p.Key, cfg: cfg, stats: stats}
}

func (c *ConfirmationGuardPlugin) Name() string { return c.name }

func (c *ConfirmationGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(c.beforeTool)
}

func (c *ConfirmationGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if !c.needsConfirm(args) {
		c.record(ctx, "before_tool", "ok")
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if strings.EqualFold(strings.TrimSpace(c.cfg.DefaultAction), "allow") {
		c.record(ctx, "before_tool", "ok")
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	c.record(ctx, "before_tool", "blocked")
	msg := fmt.Sprintf("confirmation_guard: tool %q requires confirmation (no human channel; default_action=reject)", args.ToolName)
	return &trpctool.BeforeToolResult{
		Context:      ctx,
		CustomResult: map[string]any{"error": msg, "blocked": true},
	}, nil
}

func (c *ConfirmationGuardPlugin) needsConfirm(args *trpctool.BeforeToolArgs) bool {
	if toolInList(args.ToolName, c.cfg.ConfirmTools) {
		return true
	}
	if len(c.cfg.ConfirmPatterns) == 0 {
		return false
	}
	raw := string(args.Arguments)
	if raw == "" {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(args.Arguments, &m); err != nil {
		return containsAny(raw, c.cfg.ConfirmPatterns)
	}
	b, _ := json.Marshal(m)
	return containsAny(string(b), c.cfg.ConfirmPatterns)
}

func (c *ConfirmationGuardPlugin) record(ctx context.Context, point, status string) {
	if c.stats != nil {
		c.stats.Record(ctx, c.name, point, status)
	}
}
