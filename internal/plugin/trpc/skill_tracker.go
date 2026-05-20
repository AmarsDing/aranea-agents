package plugintrpc

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type skillUsageConfig struct {
	TrackAllTools      bool     `json:"track_all_tools"`
	TrackedTools       []string `json:"tracked_tools"`
	MaxCallsPerSession int      `json:"max_calls_per_session"`
}

type SkillUsageTrackerPlugin struct {
	name   string
	cfg    skillUsageConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*SkillUsageTrackerPlugin)(nil)

func NewSkillUsageTrackerPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *SkillUsageTrackerPlugin {
	var cfg skillUsageConfig
	cfg.TrackAllTools = true
	cfg.MaxCallsPerSession = 100
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	name := p.Key
	return &SkillUsageTrackerPlugin{
		name:   name,
		cfg:    cfg,
		stats:  stats,
		logger: NewPluginSafeLogger(name, bus),
	}
}

func (s *SkillUsageTrackerPlugin) Name() string { return s.name }

func (s *SkillUsageTrackerPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(s.beforeTool)
	r.AfterTool(s.afterTool)
}

func (s *SkillUsageTrackerPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if !s.shouldTrack(args.ToolName) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	s.logger.Info("plugin.skill_tracker.before_tool",
		"tool", args.ToolName,
	)
	s.record(ctx, "before_tool", "ok")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (s *SkillUsageTrackerPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil {
		return &trpctool.AfterToolResult{}, nil
	}
	if !s.shouldTrack(args.ToolName) {
		return &trpctool.AfterToolResult{}, nil
	}
	status := "ok"
	if args.Error != nil {
		status = "error"
	}
	s.logger.Info("plugin.skill_tracker.after_tool",
		"tool", args.ToolName,
		"status", status,
	)
	s.record(ctx, "after_tool", status)
	return &trpctool.AfterToolResult{}, nil
}

func (s *SkillUsageTrackerPlugin) shouldTrack(toolName string) bool {
	if s.cfg.TrackAllTools {
		return true
	}
	return toolInList(toolName, s.cfg.TrackedTools)
}

func (s *SkillUsageTrackerPlugin) record(ctx context.Context, point, status string) {
	if s.stats != nil {
		s.stats.Record(ctx, s.name, point, status)
	}
}
