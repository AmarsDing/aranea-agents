package preview

import (
	"regexp"
	"strings"
)

var (
	reThinkingBlock      = regexp.MustCompile(`(?is)<think(?:ing)?>.*?</think(?:ing)?>`)
	reRedactedBlock      = regexp.MustCompile(`(?is)<think>.*?</think>`)
	reThinkingTag        = regexp.MustCompile(`(?is)</?think(?:ing)?>`)
	reRedactedThinking   = regexp.MustCompile(`(?is)</?redacted_thinking>`)
	reMalformedToolXML   = regexp.MustCompile(`(?is)</?(?:tool|function_call|invoke)[^>]*>`)
)

// SanitizeStreamText strips thinking tags and malformed tool XML from stream deltas (CH-BOR-12).
func SanitizeStreamText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = reThinkingBlock.ReplaceAllString(text, "")
	text = reRedactedBlock.ReplaceAllString(text, "")
	text = reThinkingTag.ReplaceAllString(text, "")
	text = reRedactedThinking.ReplaceAllString(text, "")
	text = reMalformedToolXML.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// SanitizeIMVisibleText applies stream sanitize plus blank-line normalization for outbound IM text.
func SanitizeIMVisibleText(text string) string {
	text = SanitizeStreamText(text)
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
