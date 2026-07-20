package session

import (
	"errors"
	"strings"
	"unicode/utf8"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// 质量门常量（参考 Grok xai-grok-compaction code_compaction/config.rs）：
const (
	// minSummarySeedChars: 清洗后摘要低于此 rune 数视为退化（Grok=500；本系统
	// 摘要面向中文短会话，取 200）。
	minSummarySeedChars = 200
	// minTranscriptCharsForGuard: 原文不足此 rune 数时不启用退化判定
	// （短对话本就只能产出短摘要）。
	minTranscriptCharsForGuard = 1000
	// maxSummaryReductionRatio: 摘要 token 估算 ≥ 原文 80% 视为无效压缩，丢弃结果。
	maxSummaryReductionRatio = 0.8
	// llmCompressMaxAttempts: 空响应/退化摘要的重试上限（首次 + 1 次重试）。
	llmCompressMaxAttempts = 2
)

// isDegenerateSummary reports whether the LLM summary is too short to be a
// faithful compression of a substantial transcript (pure function).
func isDegenerateSummary(md string, transcriptRunes int) bool {
	if transcriptRunes < minTranscriptCharsForGuard {
		return false
	}
	return utf8.RuneCountInString(strings.TrimSpace(md)) < minSummarySeedChars
}

// passesReductionGuard reports whether the compression materially reduced
// tokens (pure function). Zero inputs pass (nothing meaningful to compare).
func passesReductionGuard(summaryTokens, bodyTokens int) bool {
	if summaryTokens <= 0 || bodyTokens <= 0 {
		return true
	}
	return float64(summaryTokens) < maxSummaryReductionRatio*float64(bodyTokens)
}

// classifyCompressError maps a compressor error to a failure kind.
// Pure logic only — no I/O, no logging（与 Grok classify_error 同纪律）.
func classifyCompressError(err error) compressFailureKind {
	if err == nil {
		return compressFailureNone
	}
	var respErr *trpcmodel.ResponseError
	if errors.As(err, &respErr) && respErr != nil {
		msg := strings.ToLower(respErr.Message)
		// 上下文溢出：确定性失败，重发必然再败（Grok: 永远 Fatal）。
		if strings.Contains(msg, "context length") ||
			strings.Contains(msg, "maximum context") ||
			strings.Contains(msg, "context_window") ||
			strings.Contains(msg, "too many tokens") ||
			strings.Contains(msg, "reduce the length") {
			return compressFailureDeterministic
		}
		if respErr.Code != nil {
			switch strings.ToLower(*respErr.Code) {
			case "context_length_exceeded", "invalid_api_key", "model_not_found":
				return compressFailureDeterministic
			}
		}
		switch strings.ToLower(respErr.Type) {
		case "invalid_request_error", "authentication_error", "permission_error":
			return compressFailureDeterministic
		case "rate_limit_error", "server_error", "timeout", "overloaded_error":
			return compressFailureTransient
		}
	}
	// 未知错误按瞬态处理（允许重试，由失败抑制控制频率）。
	return compressFailureTransient
}
