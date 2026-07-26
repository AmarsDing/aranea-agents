package team

import (
	"strings"

	"aranea-agents/internal/agent"
)

// stepTokensForMember picks per-member tokens from the event stream, with anchor fallback.
// The third return value is the cache-hit portion of the prompt tokens.
func stepTokensForMember(agentKey string, sortIdx int, result agent.EventStreamResult, fallbackIn, fallbackOut, fallbackCached int) (int, int, int) {
	key := strings.TrimSpace(agentKey)
	if result.MemberUsage != nil {
		if u, ok := result.MemberUsage[key]; ok && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
			return u.PromptTokens, u.CompletionTokens, u.CachedTokens
		}
	}
	if sortIdx == 0 {
		return fallbackIn, fallbackOut, fallbackCached
	}
	return 0, 0, 0
}
