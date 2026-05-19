package plugintrpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type skillTrackerConfig struct {
	TrackSuccess          bool `json:"track_success"`
	TrackFailure          bool `json:"track_failure"`
	CaptureInputPreview     bool `json:"capture_input_preview"`
	CaptureOutputPreview    bool `json:"capture_output_preview"`
	MaxPreviewLength        int  `json:"max_preview_length"`
}

type SkillUsageTrackerPlugin struct {
	name  string
	cfg   skillTrackerConfig
	stats StatsRecorder
}

var _ trpcplugin.Plugin = (*SkillUsageTrackerPlugin)(nil)

func NewSkillUsageTrackerPlugin(p biz.Plugin, stats StatsRecorder) *SkillUsageTrackerPlugin {
	var cfg skillTrackerConfig
	cfg.TrackSuccess = true
	cfg.TrackFailure = true
	cfg.CaptureInputPreview = true
	cfg.CaptureOutputPreview = true
	cfg.MaxPreviewLength = 500
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &SkillUsageTrackerPlugin{name: p.Key, cfg: cfg, stats: stats}
}

func (s *SkillUsageTrackerPlugin) Name() string { return s.name }

func (s *SkillUsageTrackerPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(s.beforeTool)
	r.AfterTool(s.afterTool)
}

func (s *SkillUsageTrackerPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args != nil && s.cfg.CaptureInputPreview {
		preview := truncateString(string(args.Arguments), s.cfg.MaxPreviewLength)
		sid, akey := sessionAgentKey(ctx, nil)
		slog.Info("skill_usage.before_tool",
			"plugin", s.name,
			"tool", args.ToolName,
			"session_id", sid,
			"agent_key", akey,
			"args_preview", preview,
		)
	}
	s.record(ctx, "before_tool", "ok")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (s *SkillUsageTrackerPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	status := "ok"
	if args != nil && args.Error != nil {
		status = "error"
	}
	if args != nil {
		if (status == "ok" && s.cfg.TrackSuccess) || (status == "error" && s.cfg.TrackFailure) {
			out := ""
			if s.cfg.CaptureOutputPreview && args.Result != nil {
				out = truncateString(fmtAny(args.Result), s.cfg.MaxPreviewLength)
			}
			sid, akey := sessionAgentKey(ctx, nil)
			slog.Info("skill_usage.after_tool",
				"plugin", s.name,
				"tool", args.ToolName,
				"session_id", sid,
				"agent_key", akey,
				"status", status,
				"output_preview", out,
				"at", time.Now().UTC().Format(time.RFC3339),
			)
		}
	}
	s.record(ctx, "after_tool", status)
	return &trpctool.AfterToolResult{}, nil
}

func fmtAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *SkillUsageTrackerPlugin) record(ctx context.Context, point, status string) {
	if s.stats != nil {
		s.stats.Record(ctx, s.name, point, status)
	}
}
