package session

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

func roughTokenEstimate(s string) int {
	return llmcontext.RoughTokenEstimate(s)
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
