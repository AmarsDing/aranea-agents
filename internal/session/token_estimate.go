package session

import (
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

// roughTokenEstimate estimates tokens for compression budget decisions via the
// shared calibrated estimator (default blended 2.5 chars/token, recalibrated
// by provider-reported usage). Display-only estimates keep using
// llmcontext.RoughTokenEstimate instead.
func roughTokenEstimate(s string) int {
	return llmcontext.EstimateTokensFromChars(utf8.RuneCountInString(strings.TrimSpace(s)))
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
