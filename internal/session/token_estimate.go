package session

import (
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
)

func roughTokenEstimate(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := utf8.RuneCountInString(s)
	if n < 1 {
		return 0
	}
	est := n / 4
	if est < 1 {
		return 1
	}
	return est
}

func estimateCompactedPromptTokens(mergedSummary string, tail []biz.ChatMessage) int {
	var b strings.Builder
	b.WriteString(mergedSummary)
	b.WriteString("\n")
	for _, m := range tail {
		b.WriteString(m.ContentMarkdown)
		b.WriteString("\n")
	}
	return roughTokenEstimate(b.String())
}
