package jobs

import "testing"

// S1: predictive heal is wired with a NoopHealActionHandler, so the job must
// be opt-in only — default disabled to avoid misleading "applied" records.
func TestPredictiveHealJobEnabled_DefaultDisabled(t *testing.T) {
	t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", "")
	if PredictiveHealJobEnabled() {
		t.Fatal("expected disabled by default")
	}
}

func TestPredictiveHealJobEnabled_OptIn(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes"} {
		t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", v)
		if !PredictiveHealJobEnabled() {
			t.Fatalf("expected enabled for %q", v)
		}
	}
	t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", "0")
	if PredictiveHealJobEnabled() {
		t.Fatal("expected disabled for \"0\"")
	}
}
