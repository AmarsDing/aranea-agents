package preview

import (
	"encoding/json"
	"fmt"
	"strings"
)

const cardOneLineSummaryRunes = 48

// BuildFeishuToolCardJSON builds a compact single-line Feishu interactive card.
func BuildFeishuToolCardJSON(seg Segment, opts ToolCardBuildOpts) (string, error) {
	vis := resolveToolVisual(seg.Meta.ActivityKind, seg.Status)
	line := formatToolCardOneLineLarkMD(seg, vis)
	detailURL := BuildSessionWebURL(opts.WebOrigin, opts.SessionID, seg.ID)

	var columns []any
	columns = append(columns, map[string]any{
		"tag":            "column",
		"width":          "weighted",
		"weight":         5,
		"vertical_align": "center",
		"elements": []any{
			map[string]any{
				"tag": "div",
				"text": map[string]any{
					"tag":     "lark_md",
					"content": line,
				},
			},
		},
	})
	if detailURL != "" {
		columns = append(columns, map[string]any{
			"tag":            "column",
			"width":          "auto",
			"vertical_align": "center",
			"elements": []any{
				map[string]any{
					"tag": "action",
					"actions": []any{
						map[string]any{
							"tag":  "button",
							"type": "default",
							"text": map[string]any{
								"tag":     "plain_text",
								"content": "Web 详情",
							},
							"url": detailURL,
						},
					},
				},
			},
		})
	}

	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": cardHeaderTemplate(seg.Meta.ActivityKind, seg.Status),
			"title": map[string]any{
				"tag":     "plain_text",
				"content": cardHeaderTitle(seg, vis),
			},
		},
		"elements": []any{
			map[string]any{
				"tag":        "column_set",
				"flex_mode":  "none",
				"horizontal_spacing": "8px",
				"columns":    columns,
			},
		},
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func cardHeaderTemplate(activityKind, status string) string {
	if ToolStatusInFlight(status) {
		return "orange"
	}
	if strings.EqualFold(strings.TrimSpace(status), ToolStatusError) {
		return "red"
	}
	if strings.EqualFold(strings.TrimSpace(status), ToolStatusOK) || isTerminalSuccess(status) {
		return "green"
	}
	vis := resolveToolVisual(activityKind, status)
	return vis.Template
}

func isTerminalSuccess(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ToolStatusOK, "done", "success", "":
		return true
	default:
		return false
	}
}

func cardHeaderTitle(seg Segment, vis toolVisualStyle) string {
	label := strings.TrimSpace(seg.Meta.DisplayLabel)
	if label == "" {
		label = strings.TrimSpace(seg.Meta.Name)
	}
	if label == "" {
		label = "工具"
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s · %s", vis.Emoji, vis.KindLabel, label))
}

func formatToolCardOneLineLarkMD(seg Segment, vis toolVisualStyle) string {
	label := toolDisplayLabel(seg.Meta)
	summary := compactToolSummary(seg)
	statusMD := toolStatusMarkdown(seg.Status)
	parts := []string{
		vis.Emoji,
		fmt.Sprintf("**%s**", vis.KindLabel),
		escapeLarkMD(label),
	}
	if summary != "" {
		parts = append(parts, "`"+escapeLarkMD(summary)+"`")
	}
	parts = append(parts, statusMD)
	if d := formatDurationMS(seg.Meta.DurationMS); d != "" && !ToolStatusInFlight(seg.Status) {
		parts[len(parts)-1] = statusMD + " " + d
	}
	if code := strings.TrimSpace(seg.Meta.ErrorCode); code != "" {
		parts = append(parts, "`"+escapeLarkMD(code)+"`")
	}
	return strings.Join(parts, " · ")
}

// FormatToolOneLinePlain renders the same semantics for IM preview text (no HTML).
func FormatToolOneLinePlain(seg Segment) string {
	vis := resolveToolVisual(seg.Meta.ActivityKind, seg.Status)
	label := toolDisplayLabel(seg.Meta)
	summary := compactToolSummary(seg)
	icon, statusLabel := toolStatusPlain(seg.Status)
	parts := []string{vis.Emoji, vis.KindLabel, label}
	if summary != "" {
		parts = append(parts, summary)
	}
	line := strings.Join(parts, " · ")
	line += fmt.Sprintf(" · %s %s", icon, statusLabel)
	if d := formatDurationMS(seg.Meta.DurationMS); d != "" && !ToolStatusInFlight(seg.Status) {
		line += " · " + d
	}
	return line
}

func toolDisplayLabel(meta ToolSegmentMeta) string {
	if s := strings.TrimSpace(meta.DisplayLabel); s != "" {
		return s
	}
	if s := strings.TrimSpace(meta.Name); s != "" {
		return s
	}
	return "工具"
}

func compactToolSummary(seg Segment) string {
	kind := strings.ToLower(strings.TrimSpace(seg.Meta.ActivityKind))
	summary := strings.TrimSpace(seg.Meta.Summary)
	switch kind {
	case "mcp":
		if summary == "" {
			summary = strings.TrimSpace(seg.Meta.Name)
		}
	case "skill":
		if summary == "" {
			summary = strings.TrimSpace(seg.Meta.Name)
		}
	case "knowledge", "memory":
		if summary == "" && seg.Meta.ResultExcerpt != "" {
			summary = seg.Meta.ResultExcerpt
		}
	case "subagent":
		if summary == "" {
			summary = strings.TrimSpace(seg.Meta.Name)
		}
	default:
		if summary == "" {
			summary = strings.TrimSpace(seg.Meta.Name)
		}
	}
	if strings.EqualFold(strings.TrimSpace(seg.Status), ToolStatusError) {
		if excerpt := strings.TrimSpace(seg.Meta.ResultExcerpt); excerpt != "" {
			summary = excerpt
		}
	}
	return truncateRunes(summary, cardOneLineSummaryRunes)
}

func toolStatusMarkdown(status string) string {
	if ToolStatusInFlight(status) {
		return "<font color='orange'>🔄 进行中</font>"
	}
	if strings.EqualFold(strings.TrimSpace(status), ToolStatusError) {
		return "<font color='red'>✕ 失败</font>"
	}
	return "<font color='green'>✓ 完成</font>"
}

func toolStatusPlain(status string) (icon, label string) {
	if ToolStatusInFlight(status) {
		return "🔄", "进行中"
	}
	if strings.EqualFold(strings.TrimSpace(status), ToolStatusError) {
		return "✕", "失败"
	}
	return "✓", "完成"
}
