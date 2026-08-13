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

// estimateCompactedPromptTokens 估算压缩后的 prompt token 数。语义必须与压缩前
// ContextUsedTokens（provider 上报的 prompt_tokens = 系统提示 + 工具 schema + 内容）
// 一致，因此计入 reservedSystem（不可压缩的系统部分）——否则触发逻辑在下一次
// 权威更新前一直运行在偏低的估值上（压缩后立刻又软触发的抖动）。
func estimateCompactedPromptTokens(mergedSummary string, tail []biz.ChatMessage, reservedSystem int) int {
	var b strings.Builder
	b.WriteString(mergedSummary)
	b.WriteString("\n")
	for _, m := range tail {
		b.WriteString(m.ContentMarkdown)
		b.WriteString("\n")
	}
	return reservedSystem + roughTokenEstimate(b.String())
}
