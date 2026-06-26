package preview

import (
	"fmt"
	"strings"
)

type toolVisualStyle struct {
	Emoji     string
	KindLabel string
	Template  string
}

func resolveToolVisual(activityKind, status string) toolVisualStyle {
	kind := strings.ToLower(strings.TrimSpace(activityKind))
	vis := toolVisualStyle{
		Emoji:     "🔧",
		KindLabel: "工具",
		Template:  "blue",
	}
	switch kind {
	case "mcp":
		vis.Emoji, vis.KindLabel, vis.Template = "📡", "MCP", "turquoise"
	case "skill":
		vis.Emoji, vis.KindLabel, vis.Template = "⚡", "Skill", "purple"
	case "subagent":
		vis.Emoji, vis.KindLabel, vis.Template = "🤖", "Agent", "indigo"
	case "knowledge":
		vis.Emoji, vis.KindLabel, vis.Template = "📚", "知识库", "wathet"
	case "memory":
		vis.Emoji, vis.KindLabel, vis.Template = "🧠", "记忆", "grey"
	case "session":
		vis.Emoji, vis.KindLabel, vis.Template = "💬", "会话", "blue"
	}
	switch NormalizeToolStatus(status) {
	case ToolStatusCalling:
		vis.Template = "orange"
	case ToolStatusError:
		vis.Template = "red"
	case ToolStatusOK, "":
		if vis.Template == "blue" {
			vis.Template = "green"
		}
	}
	return vis
}

func toolStatusBadge(status string) (icon, label string) {
	if ToolStatusInFlight(status) {
		return "⏳", "执行中"
	}
	if IsToolStatusError(status) {
		return "✗", "失败"
	}
	return "✓", "完成"
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := float64(ms) / 1000
	if sec < 10 {
		return fmt.Sprintf("%.1fs", sec)
	}
	return fmt.Sprintf("%.0fs", sec)
}

func escapeLarkMD(s string) string {
	replacer := strings.NewReplacer(
		"`", "'",
		"*", "×",
		"[", "(",
		"]", ")",
	)
	return replacer.Replace(s)
}

func toolCardFooterName(meta ToolSegmentMeta, segID string) string {
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = strings.TrimSpace(segID)
	}
	return name
}
