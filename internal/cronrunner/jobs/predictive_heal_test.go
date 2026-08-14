package jobs

import "testing"

// The predictive heal job is wired with the real CatalogHealActionHandler and
// a metric-driven confidence gate, so it is enabled by default (opt-out).
func TestPredictiveHealJobEnabled_DefaultEnabled(t *testing.T) {
	t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", "")
	if !PredictiveHealJobEnabled() {
		t.Fatal("expected enabled by default")
	}
}

func TestPredictiveHealJobEnabled_OptOut(t *testing.T) {
	for _, v := range []string{"0", "false", "FALSE", "no"} {
		t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", v)
		if PredictiveHealJobEnabled() {
			t.Fatalf("expected disabled for %q", v)
		}
	}
	t.Setenv("PREDICTIVE_HEAL_JOB_ENABLED", "1")
	if !PredictiveHealJobEnabled() {
		t.Fatal("expected enabled for \"1\"")
	}
}
