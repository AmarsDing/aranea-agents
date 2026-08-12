package event

import "testing"

// 75 M1.4：computeruse 流程日志 step_id 必须登记中文标题（红线：禁止无标题发射）。
func TestStepTitleRegistry_ComputerUseSteps(t *testing.T) {
	steps := []string{
		"computeruse.session.start",
		"computeruse.session.done",
		"computeruse.act",
		"computeruse.act.done",
		"computeruse.act.error",
		"computeruse.grounding.fallback",
		"computeruse.budget.exceeded",
		"computeruse.killswitch",
	}
	for _, id := range steps {
		if got := stepTitle(id); got == id {
			t.Errorf("step %q has no registered title", id)
		}
	}
}
