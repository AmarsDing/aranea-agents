package plugintrpc

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type skillUsageConfig struct {
	TrackSuccess         bool `json:"track_success"`
	TrackFailure         bool `json:"track_failure"`
	CaptureInputPreview  bool `json:"capture_input_preview"`
	CaptureOutputPreview bool `json:"capture_output_preview"`
	MaxPreviewLength     int  `json:"max_preview_length"`
}

type SkillUsageTrackerPlugin struct {
	base basePlugin
	cfg  skillUsageConfig
}

var _ trpcplugin.Plugin = (*SkillUsageTrackerPlugin)(nil)

func NewSkillUsageTrackerPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger) *SkillUsageTrackerPlugin {
	var cfg skillUsageConfig
	cfg.TrackSuccess = true
	cfg.TrackFailure = true
	cfg.CaptureInputPreview = true
	cfg.CaptureOutputPreview = true
	cfg.MaxPreviewLength = 500
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	if cfg.MaxPreviewLength <= 0 {
		cfg.MaxPreviewLength = 500
	}
	return &SkillUsageTrackerPlugin{
		base: newBasePlugin(p.Key, stats, monitorBus, lg),
		cfg:  cfg,
	}
}

func (s *SkillUsageTrackerPlugin) Name() string { return s.base.Name() }

func (s *SkillUsageTrackerPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(s.beforeTool)
	r.AfterTool(s.afterTool)
}

func isSkillTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "use_skill", "skill_search":
		return true
	}
	return strings.HasPrefix(name, "skill_")
}

func (s *SkillUsageTrackerPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil || !isSkillTool(args.ToolName) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if s.cfg.CaptureInputPreview {
		preview := truncateString(redactText(string(args.Arguments), true, true, true), s.cfg.MaxPreviewLength)
		s.base.logger.Info("plugin.skill_tracker.before_tool",
			"tool", args.ToolName,
			"input_preview", preview,
		)
	}
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (s *SkillUsageTrackerPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil || !isSkillTool(args.ToolName) {
		return &trpctool.AfterToolResult{}, nil
	}
	status := "success"
	if args.Error != nil {
		status = "error"
	}
	if status == "error" && !s.cfg.TrackFailure {
		return &trpctool.AfterToolResult{}, nil
	}
	if status == "success" && !s.cfg.TrackSuccess {
		return &trpctool.AfterToolResult{}, nil
	}
	fields := []any{"tool", args.ToolName, "status", status}
	if s.cfg.CaptureOutputPreview && args.Result != nil {
		fields = append(fields, "output_preview", truncateString(redactText(formatToolResult(args.Result), true, true, true), s.cfg.MaxPreviewLength))
	}
	s.base.logger.Info("plugin.skill_tracker.after_tool", fields...)
	// Record asynchronously to avoid blocking the tool execution pipeline
	//（session/agent 需在 request ctx 存活时同步提取）
	sid, akey := sessionAgentKey(ctx, nil)
	summary := ""
	if args.Error != nil {
		summary = "skill tool " + args.ToolName + " failed: " + args.Error.Error()
	}
	capturedCtx := context.Background()
	statusCapture := status
	safego.Go(capturedCtx, "skill_tracker.after_tool.record", func() {
		s.base.recordEventAt(sid, akey, "after_tool", statusCapture, summary)
	})
	return &trpctool.AfterToolResult{}, nil
}

func formatToolResult(result any) string {
	if result == nil {
		return ""
	}
	switch v := result.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
