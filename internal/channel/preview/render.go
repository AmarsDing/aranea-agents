package preview

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// RenderPlainText formats transcript segments for IM delivery.
func RenderPlainText(t *Transcript, policy biz.ChannelIMRenderPolicy) string {
	if t == nil {
		return ""
	}
	segs := t.Segments()
	if len(segs) == 0 {
		return ""
	}

	mode := strings.TrimSpace(policy.Mode)
	if mode == "" {
		mode = biz.ChannelIMRenderModeReplyOnly
	}

	switch mode {
	case biz.ChannelIMRenderModeReplyOnly:
		return truncateRunes(lastTextContent(segs), policy.MaxPreviewRunes)
	default:
		return truncateTailRunes(renderTranscript(segs, policy), policy.MaxPreviewRunes)
	}
}

func renderTranscript(segs []Segment, policy biz.ChannelIMRenderPolicy) string {
	var b strings.Builder
	showReasoning := policy.ShowReasoning || policy.Mode == biz.ChannelIMRenderModeTranscriptWithReasoning
	for _, seg := range segs {
		line := renderSegment(seg, policy, showReasoning)
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}

func renderSegment(seg Segment, policy biz.ChannelIMRenderPolicy, showReasoning bool) string {
	switch seg.Kind {
	case SegmentSystem:
		return strings.TrimSpace(seg.Content)
	case SegmentReasoning:
		if !showReasoning {
			return ""
		}
		text := truncateRunes(strings.TrimSpace(seg.Content), policy.ReasoningMaxRunes)
		if text == "" {
			return ""
		}
		return "💭 " + text
	case SegmentText:
		return strings.TrimSpace(seg.Content)
	case SegmentTool:
		return renderToolSegment(seg, policy)
	case SegmentMember:
		return renderMemberSegment(seg, policy)
	default:
		return ""
	}
}

func renderToolSegment(seg Segment, policy biz.ChannelIMRenderPolicy) string {
	if policy.ToolDetail == biz.ChannelIMToolDetailOff {
		return ""
	}
	if policy.ToolCardMode == biz.ChannelIMToolCardModeFeishuAppend {
		return renderToolSegmentCompact(seg)
	}
	label := seg.Meta.DisplayLabel
	if label == "" {
		label = seg.Meta.Name
	}
	if label == "" {
		label = "工具"
	}
	prefix := toolKindPrefix(seg.Meta.ActivityKind)
	line := prefix + label
	if policy.ToolDetail == biz.ChannelIMToolDetailLabelSummary && seg.Meta.Summary != "" {
		line += " · " + seg.Meta.Summary
	}
	switch NormalizeToolStatus(seg.Status) {
	case ToolStatusCalling:
		line += "\n  ⏳ 执行中…"
	case ToolStatusError:
		line += "\n  ✗ 失败"
	default:
		line += "\n  ✓ 完成"
	}
	return line
}

func renderToolSegmentCompact(seg Segment) string {
	return FormatToolOneLinePlain(seg)
}

func renderMemberSegment(seg Segment, policy biz.ChannelIMRenderPolicy) string {
	switch policy.TeamMode {
	case biz.ChannelIMTeamModeOff:
		return ""
	case biz.ChannelIMTeamModeSteps:
		author := strings.TrimSpace(seg.Author)
		if author == "" {
			author = "Team 成员"
		}
		text := strings.TrimSpace(seg.Content)
		if text == "" {
			return author + " 处理中…"
		}
		return author + ": " + truncateRunes(text, 120)
	default:
		author := strings.TrimSpace(seg.Author)
		text := strings.TrimSpace(seg.Content)
		if author == "" && text == "" {
			return ""
		}
		if text == "" {
			return "👤 " + author + " 处理中…"
		}
		if author == "" {
			return text
		}
		return "👤 " + author + ":\n" + text
	}
}

func toolKindPrefix(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mcp":
		return "▸ MCP · "
	case "skill":
		return "▸ Skill · "
	case "subagent":
		return "▸ Agent · "
	case "knowledge":
		return "▸ 知识库 · "
	case "memory":
		return "▸ 记忆 · "
	default:
		return "▸ "
	}
}

func lastTextContent(segs []Segment) string {
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].Kind == SegmentText {
			return strings.TrimSpace(segs[i].Content)
		}
	}
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].Kind == SegmentSystem {
			return strings.TrimSpace(segs[i].Content)
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func truncateTailRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	head := 120
	if head > max/4 {
		head = max / 4
	}
	return string(runes[:head]) + fmt.Sprintf("\n…（省略 %d 字）\n\n", len(runes)-max+head) + string(runes[len(runes)-max+head:])
}
