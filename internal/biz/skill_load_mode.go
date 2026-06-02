package biz

const (
	SkillLoadModeEager       = "eager"
	SkillLoadModeProgressive = "progressive"
)

func IsProgressiveSkillLoad(mode string) bool {
	return mode == SkillLoadModeProgressive
}
