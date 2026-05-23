package preview

import (
	"regexp"
	"strings"
)

var (
	reHTMLTag       = regexp.MustCompile(`<[^>]+>`)
	reMDHeader      = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reMDBold        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMDItalic      = regexp.MustCompile(`(?:^|[^*])\*([^*\n]+?)\*(?:[^*]|$)`)
	reMDLink        = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reMDCodeFence   = regexp.MustCompile("(?s)```(?:\\w+)?\\n?(.*?)```")
	reMDInlineCode  = regexp.MustCompile("`([^`]+)`")
	reMDTableSep    = regexp.MustCompile(`(?m)^\|?[\s\-:|]+\|?$`)
	reMDTableRow    = regexp.MustCompile(`(?m)^\|(.+)\|$`)
	reBlankLines    = regexp.MustCompile(`\n{3,}`)
	reReactTagBlock = regexp.MustCompile(`(?s)/\*[A-Z_]+\*/`)
)

const reactFinalAnswerTag = "/*FINAL_ANSWER*/"

// reactPlannerTags must stay aligned with web/src/features/chat/reactPlannerParse.ts TAG_DEFS + FINAL_TAG.
var reactPlannerTags = []string{
	"/*PLANNING*/",
	"/*REASONING*/",
	"/*ACTION*/",
	"/*REPLANNING*/",
	reactFinalAnswerTag,
}

// FormatAssistantReplyForIM formats raw assistant markdown for IM delivery (ReAct extract + markdown cleanup).
func FormatAssistantReplyForIM(platform, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	text := extractAssistantReplyBody(raw)
	text = stripA2UIJsonl(text)
	text = markdownToIMPlain(text, platform)
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// FormatRenderedTranscriptForIM formats IM preview transcript text without stripping tool/reasoning segments.
func FormatRenderedTranscriptForIM(platform, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	text := stripReactTagMarkers(raw)
	text = stripA2UIJsonl(text)
	text = markdownToIMPlain(text, platform)
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// FormatOutboundText is an alias for FormatAssistantReplyForIM.
func FormatOutboundText(platform, raw string) string {
	return FormatAssistantReplyForIM(platform, raw)
}

func extractAssistantReplyBody(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, reactFinalAnswerTag); idx >= 0 {
		return strings.TrimSpace(raw[idx+len(reactFinalAnswerTag):])
	}
	if !contentHasReactTags(raw) {
		return raw
	}
	if answer := reactFinalAnswerFromTagged(raw); answer != "" {
		return answer
	}
	return stripReactTagMarkers(raw)
}

func contentHasReactTags(text string) bool {
	for _, tag := range reactPlannerTags {
		if strings.Contains(text, tag) {
			return true
		}
	}
	return false
}

func reactFinalAnswerFromTagged(raw string) string {
	pos := 0
	finalAnswer := ""
	for pos < len(raw) {
		hit := findEarliestReactTag(raw, pos)
		if hit == nil {
			break
		}
		contentStart := hit.index + len(hit.tag)
		if hit.isFinal {
			finalAnswer = strings.TrimSpace(raw[contentStart:])
			break
		}
		next := findEarliestReactTag(raw, contentStart)
		segmentEnd := len(raw)
		if next != nil {
			segmentEnd = next.index
		}
		pos = segmentEnd
	}
	return strings.TrimSpace(finalAnswer)
}

type reactTagHit struct {
	index   int
	tag     string
	isFinal bool
}

func findEarliestReactTag(text string, from int) *reactTagHit {
	var best *reactTagHit
	for _, tag := range reactPlannerTags {
		i := strings.Index(text[from:], tag)
		if i < 0 {
			continue
		}
		idx := from + i
		if best == nil || idx < best.index {
			best = &reactTagHit{index: idx, tag: tag, isFinal: tag == reactFinalAnswerTag}
		}
	}
	return best
}

func stripReactTagMarkers(raw string) string {
	out := reReactTagBlock.ReplaceAllString(raw, "")
	return strings.TrimSpace(out)
}

func stripA2UIJsonl(raw string) string {
	lines := strings.Split(raw, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			filtered = append(filtered, "")
			continue
		}
		if isA2UIJsonLine(trimmed) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func isA2UIJsonLine(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "{") || !strings.Contains(trimmed, `"type"`) {
		return false
	}
	return strings.Contains(trimmed, `"surfaceUpdate"`) ||
		strings.Contains(trimmed, `"surface":`) ||
		strings.Contains(trimmed, `"beginRendering"`)
}

func markdownToIMPlain(md, platform string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	text := md
	text = reMDCodeFence.ReplaceAllStringFunc(text, func(s string) string {
		inner := reMDCodeFence.FindStringSubmatch(s)
		if len(inner) < 2 {
			return ""
		}
		body := strings.TrimSpace(inner[1])
		if body == "" {
			return ""
		}
		return "\n「\n" + body + "\n」\n"
	})
	text = reMDInlineCode.ReplaceAllString(text, "$1")
	text = reMDLink.ReplaceAllString(text, "$1 ($2)")
	text = reMDHeader.ReplaceAllString(text, "【$1】")
	text = reMDBold.ReplaceAllString(text, "$1")
	text = reMDItalic.ReplaceAllString(text, "$1")
	text = reHTMLTag.ReplaceAllString(text, "")
	text = formatMarkdownTables(text)
	text = normalizeListMarkers(text)
	if p := strings.ToLower(strings.TrimSpace(platform)); p == "feishu" || p == "lark" {
		text = strings.ReplaceAll(text, "\t", "  ")
	}
	return strings.TrimSpace(text)
}

func formatMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if reMDTableSep.MatchString(trimmed) {
			continue
		}
		if m := reMDTableRow.FindStringSubmatch(trimmed); len(m) == 2 {
			cells := strings.Split(m[1], "|")
			parts := make([]string, 0, len(cells))
			for _, c := range cells {
				c = strings.TrimSpace(c)
				if c != "" {
					parts = append(parts, c)
				}
			}
			if len(parts) > 0 {
				out = append(out, "• "+strings.Join(parts, " | "))
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func normalizeListMarkers(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- "):
			lines[i] = "• " + strings.TrimSpace(trimmed[2:])
		case strings.HasPrefix(trimmed, "* "):
			lines[i] = "• " + strings.TrimSpace(trimmed[2:])
		case len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && strings.Contains(trimmed, ". "):
			if dot := strings.Index(trimmed, ". "); dot > 0 {
				lines[i] = trimmed[:dot+1] + " " + strings.TrimSpace(trimmed[dot+2:])
			}
		}
	}
	return strings.Join(lines, "\n")
}
