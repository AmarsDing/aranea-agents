package preview

import (
	"encoding/json"
	"fmt"
	"strings"
)

const cardResultExcerptRunes = 280

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

func excerptToolResult(resultJSON, status string, maxRunes int) string {
	raw := strings.TrimSpace(resultJSON)
	if raw == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = cardResultExcerptRunes
	}
	if IsToolStatusError(status) {
		if msg := extractJSONErrorMessage(raw); msg != "" {
			return truncateRunes(msg, maxRunes)
		}
	}
	compact := compactJSONOneLine(raw)
	return truncateRunes(compact, maxRunes)
}

func extractJSONErrorMessage(raw string) string {
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return ""
	}
	for _, key := range []string{"error", "message", "msg", "detail"} {
		if v, ok := obj[key]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func compactJSONOneLine(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
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
