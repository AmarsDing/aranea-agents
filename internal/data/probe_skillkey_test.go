package data

import (
	"testing"

	"aranea-agents/internal/data/ent/platformskill"
)

// 临时探针：定位 TestGetSkillHealth 中 PlatformSkillCreate.check 调用 nil validator 的原因。
func TestProbeSkillKeyValidatorNonNil(t *testing.T) {
	if platformskill.SkillKeyValidator == nil {
		t.Fatal("SkillKeyValidator is nil in test process")
	}
	if platformskill.NameValidator == nil {
		t.Fatal("NameValidator is nil in test process")
	}
}
