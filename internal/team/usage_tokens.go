package team

import (
	"strings"

	"aranea-agents/internal/agent"
)

// stepTokensForMember picks per-member tokens from the event stream, with anchor fallback.
// Returns (input, output, cached, cacheWrite, reasoning) — cached/write are subsets
// of input; reasoning is a subset of output (provider.NormalizeUsageTokens 口径).
func stepTokensForMember(agentKey string, sortIdx int, result agent.EventStreamResult, fallbackIn, fallbackOut, fallbackCached, fallbackWrite, fallbackReasoning int) (int, int, int, int, int) {
	key := strings.TrimSpace(agentKey)
	if result.MemberUsage != nil {
		if u, ok := result.MemberUsage[key]; ok && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
			return u.PromptTokens, u.CompletionTokens, u.CachedTokens, u.CacheWriteTokens, u.ReasoningTokens
		}
	}
	if sortIdx == 0 {
		return fallbackIn, fallbackOut, fallbackCached, fallbackWrite, fallbackReasoning
	}
	return 0, 0, 0, 0, 0
}
