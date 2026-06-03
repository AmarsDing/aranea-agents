package biz

const (
	SkillLoadModeEager       = "eager"
	// SkillLoadModeProgressive must match processor.SkillLoadModeProgressive
	// and llmagent.SkillLoadModeProgressive (re-exported from processor).
	SkillLoadModeProgressive = "progressive"
)

func IsProgressiveSkillLoad(mode string) bool {
	return mode == SkillLoadModeProgressive
}
