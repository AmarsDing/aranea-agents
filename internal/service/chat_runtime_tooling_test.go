package service

import "testing"

func TestSkillRuntime_healthProvider_nilInterface(t *testing.T) {
	t.Parallel()
	var rt RuntimeTooling
	if got := rt.Skill.healthProvider(); got != nil {
		t.Fatal("zero SkillRuntime must yield a nil HealthMetricsProvider interface")
	}
}
