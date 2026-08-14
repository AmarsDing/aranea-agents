package heal

import "time"

// Test hooks exported only for tests (mirrors the historical monitor package hooks).

func ParseMetadataJSON(raw string) map[string]any {
	return parseMetadataJSON(raw)
}

func NonEmpty(ss ...string) []string {
	return nonEmpty(ss...)
}

func MatchStepID(pattern, stepID string) bool {
	return matchStepID(pattern, stepID)
}

func MatchPrerequisite(pre Prerequisite, metadata map[string]any) bool {
	return matchPrerequisite(pre, metadata)
}

// SetCooldownForTest allows tests to manipulate the cooldown state of PredictiveHealUsecase.
func (uc *PredictiveHealUsecase) SetCooldownForTest(actionType string, t time.Time) {
	uc.setCooldown(actionType, t)
}
