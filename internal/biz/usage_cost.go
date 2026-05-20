package biz

import "strings"

// NormalizeUsageStatus maps legacy writer values to DB canonical status.
func NormalizeUsageStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "success", "ok":
		return "success"
	case "failed", "fail", "error":
		return "failed"
	case "timeout", "timed_out":
		return "timeout"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(status)
	}
}

// ApplyTokenUsageCosts fills per-kind costs and total from token counts and per-1k prices.
// Skips fields already set on the event. See docs/需求/29 token.design.md §4.4.
func ApplyTokenUsageCosts(e *TokenUsageEvent) {
	if e == nil {
		return
	}
	if e.InputCostMicroUSD == 0 && e.InputTokens > 0 && e.InputPriceMicroUSDPer1K > 0 {
		e.InputCostMicroUSD = int64(e.InputTokens) * e.InputPriceMicroUSDPer1K / 1000
	}
	if e.OutputCostMicroUSD == 0 && e.OutputTokens > 0 && e.OutputPriceMicroUSDPer1K > 0 {
		e.OutputCostMicroUSD = int64(e.OutputTokens) * e.OutputPriceMicroUSDPer1K / 1000
	}
	if e.CachedInputCostMicroUSD == 0 && e.CachedInputTokens > 0 && e.CachedInputPriceMicroUSDPer1K > 0 {
		e.CachedInputCostMicroUSD = int64(e.CachedInputTokens) * e.CachedInputPriceMicroUSDPer1K / 1000
	}
	if e.ReasoningCostMicroUSD == 0 && e.ReasoningTokens > 0 && e.ReasoningPriceMicroUSDPer1K > 0 {
		e.ReasoningCostMicroUSD = int64(e.ReasoningTokens) * e.ReasoningPriceMicroUSDPer1K / 1000
	}
	if e.EmbeddingCostMicroUSD == 0 && e.EmbeddingTokens > 0 && e.EmbeddingPriceMicroUSDPer1K > 0 {
		e.EmbeddingCostMicroUSD = int64(e.EmbeddingTokens) * e.EmbeddingPriceMicroUSDPer1K / 1000
	}
	if e.TotalCostMicroUSD == 0 {
		e.TotalCostMicroUSD = e.InputCostMicroUSD + e.OutputCostMicroUSD +
			e.CachedInputCostMicroUSD + e.ReasoningCostMicroUSD + e.EmbeddingCostMicroUSD
	}
}
