package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

// memorySummaryMaxTokens is the resident <memory_summary> budget (C1 / Codex
// memory_summary contract). Profile cards already target a compact distillate;
// this cap is the prompt-side hard stop so a long card or pinned fallback
// cannot crowd out L1 / recall.
const memorySummaryMaxTokens = 800

// MemorySummaryCue renders the always-on <memory_summary> block (C1).
//
// Order:
//  1. Resident profile card when Sleep-time has distilled one.
//  2. Otherwise a compact synthesis of pinned preference/constraint facts so
//     a Spirit cold start can still state stable prefs without scanning L3.
//
// usedPinnedFallback is true only on path 2 — the inject hook then skips the
// separate pinned block to avoid duplicating the same facts. Fact IDs from
// the fallback path are returned so injected_count still tracks what the
// model actually saw. Best-effort: nil deps / empty sources yield "".
func MemorySummaryCue(
	ctx context.Context,
	reader biz.MemoryProfileCardReader,
	lister biz.MemoryPreferenceLister,
	agentID, userID string,
) (cue string, fallbackFactIDs []string, usedPinnedFallback bool) {
	if card := ProfileCardCue(ctx, reader, agentID, userID); card != "" {
		return wrapMemorySummary(card), nil, false
	}
	pinned, ids := PinnedPreferenceCueWithIDs(ctx, lister, agentID, userID)
	if pinned == "" {
		return "", nil, false
	}
	return wrapMemorySummary(pinned), ids, true
}

func wrapMemorySummary(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	wrapped := "<memory_summary>\n" + inner + "\n</memory_summary>"
	return capMemorySummaryTokens(wrapped)
}

func capMemorySummaryTokens(s string) string {
	if llmcontext.EstimateTokensFromChars(len([]rune(s))) <= memorySummaryMaxTokens {
		return s
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		cand := string(runes[:mid]) + "…"
		if llmcontext.EstimateTokensFromChars(len([]rune(cand))) <= memorySummaryMaxTokens {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo <= 0 {
		return "…"
	}
	return string(runes[:lo]) + "…"
}
