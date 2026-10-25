package data

import (
	"time"

	"aranea-agents/internal/biz"
)

func evalDurableStateKind(kind string) bool {
	switch kind {
	case "user_identity", "preference", "user_preference", "constraint", "agent_instruction":
		return true
	}
	return false
}

// A calendar query covers local midnight through the next local midnight.
// Stored state validity is [from,until), so the old value at an exact update
// boundary never counts as still valid on the following day.
func evalStateIntersectsRange(env evalCollectedItem, qa biz.EvalQueryAnalysis) bool {
	from, err := time.Parse(time.RFC3339Nano, normalizeEvalTimestamp(env.item.Timestamp))
	if err != nil {
		return false // An unknown source date cannot establish an as-of state.
	}
	if qa.RangeEnd != nil && !from.Before(qa.RangeEnd.AddDate(0, 0, 1)) {
		return false
	}
	if env.invalidUntil != "" && qa.RangeStart != nil {
		until, err := time.Parse(time.RFC3339Nano, normalizeEvalTimestamp(env.invalidUntil))
		if err != nil || !until.After(*qa.RangeStart) {
			return false
		}
	}
	return true
}
