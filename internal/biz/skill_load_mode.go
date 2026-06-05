package biz

import "strings"

const (
	// SkillLoadModeProgressive must match processor.SkillLoadModeProgressive
	// and llmagent.SkillLoadModeProgressive (re-exported from processor).
	SkillLoadModeProgressive = "progressive"
)

func IsProgressiveSkillLoad(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), SkillLoadModeProgressive)
}
