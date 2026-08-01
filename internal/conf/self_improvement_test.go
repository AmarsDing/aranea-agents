package conf

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestSelfImprovement_NilAndZeroDefaults(t *testing.T) {
	var nilConf *SelfImprovement
	zero := &SelfImprovement{}
	for name, c := range map[string]*SelfImprovement{"nil": nilConf, "zero": zero} {
		if c.SIEnabled() {
			t.Errorf("%s: SIEnabled = true, want false（默认关闭）", name)
		}
		if got := c.SIObserveInterval(); got != 15*time.Minute {
			t.Errorf("%s: SIObserveInterval = %v, want 15m", name, got)
		}
		if got := c.SIErrorClusterWindowDays(); got != 7 {
			t.Errorf("%s: SIErrorClusterWindowDays = %d, want 7", name, got)
		}
		if got := c.SIErrorClusterMinCount(); got != 5 {
			t.Errorf("%s: SIErrorClusterMinCount = %d, want 5", name, got)
		}
		if got := c.SIPerfLatencyFactor(); got != 2.0 {
			t.Errorf("%s: SIPerfLatencyFactor = %v, want 2.0", name, got)
		}
		if got := c.SIPerfTokenFactor(); got != 1.5 {
			t.Errorf("%s: SIPerfTokenFactor = %v, want 1.5", name, got)
		}
		if got := c.SIEvalRegressionThreshold(); got != 0.10 {
			t.Errorf("%s: SIEvalRegressionThreshold = %v, want 0.10", name, got)
		}
		if got := c.SITestRunsDir(); got != "" {
			t.Errorf("%s: SITestRunsDir = %q, want empty", name, got)
		}
		// Patch governance defaults (design §6.2 / D10).
		if got := c.SIMaxDiffLines(); got != 500 {
			t.Errorf("%s: SIMaxDiffLines = %d, want 500", name, got)
		}
		if got := c.SIDailyAutoApplyQuota(); got != 5 {
			t.Errorf("%s: SIDailyAutoApplyQuota = %d, want 5", name, got)
		}
		if got := c.SIMaxAttempts(); got != 3 {
			t.Errorf("%s: SIMaxAttempts = %d, want 3", name, got)
		}
		// Sandbox defaults (design §6.2 / D4).
		gt := c.SIGateTimeouts()
		if gt["g1"] != 5*time.Minute || gt["g2"] != 10*time.Minute || gt["g3"] != 5*time.Minute {
			t.Errorf("%s: SIGateTimeouts = %v, want g1=5m g2=10m g3=5m", name, gt)
		}
		if got := c.SIWorktreeRoot(); got != ".aranea-self-improve" {
			t.Errorf("%s: SIWorktreeRoot = %q, want .aranea-self-improve", name, got)
		}
	}
}

func TestSelfImprovement_ExplicitValues(t *testing.T) {
	c := &SelfImprovement{
		Enabled:         true,
		ObserveInterval: durationpb.New(5 * time.Minute),
		ErrorCluster:    &SelfImprovement_ErrorCluster{WindowDays: 3, MinCount: 10},
		Perf:            &SelfImprovement_Perf{LatencyFactor: 3.0, TokenFactor: 2.0},
		Eval:            &SelfImprovement_Eval{RegressionThreshold: 0.25},
		TestRunsDir:     "test/self-improve/test-runs",
	}
	if !c.SIEnabled() {
		t.Error("SIEnabled = false, want true")
	}
	if got := c.SIObserveInterval(); got != 5*time.Minute {
		t.Errorf("SIObserveInterval = %v, want 5m", got)
	}
	if got := c.SIErrorClusterWindowDays(); got != 3 {
		t.Errorf("SIErrorClusterWindowDays = %d, want 3", got)
	}
	if got := c.SIErrorClusterMinCount(); got != 10 {
		t.Errorf("SIErrorClusterMinCount = %d, want 10", got)
	}
	if got := c.SIPerfLatencyFactor(); got != 3.0 {
		t.Errorf("SIPerfLatencyFactor = %v, want 3.0", got)
	}
	if got := c.SIPerfTokenFactor(); got != 2.0 {
		t.Errorf("SIPerfTokenFactor = %v, want 2.0", got)
	}
	if got := c.SIEvalRegressionThreshold(); got != 0.25 {
		t.Errorf("SIEvalRegressionThreshold = %v, want 0.25", got)
	}
	if got := c.SITestRunsDir(); got != "test/self-improve/test-runs" {
		t.Errorf("SITestRunsDir = %q", got)
	}
}

func TestSelfImprovement_PatchAndSandboxExplicitValues(t *testing.T) {
	c := &SelfImprovement{
		Patch: &SelfImprovement_Patch{MaxDiffLines: 300, DailyAutoApplyQuota: 2, MaxAttempts: 5},
		Sandbox: &SelfImprovement_Sandbox{
			GateTimeouts: &SelfImprovement_Sandbox_GateTimeouts{
				G1: durationpb.New(time.Minute),
				G2: durationpb.New(2 * time.Minute),
				G3: durationpb.New(3 * time.Minute),
			},
			WorktreeRoot: ".si-sandbox",
			RepoRoot:     "test/self-improve/repo",
		},
	}
	if got := c.SIMaxDiffLines(); got != 300 {
		t.Errorf("SIMaxDiffLines = %d, want 300", got)
	}
	if got := c.SIDailyAutoApplyQuota(); got != 2 {
		t.Errorf("SIDailyAutoApplyQuota = %d, want 2", got)
	}
	if got := c.SIMaxAttempts(); got != 5 {
		t.Errorf("SIMaxAttempts = %d, want 5", got)
	}
	gt := c.SIGateTimeouts()
	if gt["g1"] != time.Minute || gt["g2"] != 2*time.Minute || gt["g3"] != 3*time.Minute {
		t.Errorf("SIGateTimeouts = %v", gt)
	}
	if got := c.SIWorktreeRoot(); got != ".si-sandbox" {
		t.Errorf("SIWorktreeRoot = %q, want .si-sandbox", got)
	}
	if got := c.SIRepoRoot(); got != "test/self-improve/repo" {
		t.Errorf("SIRepoRoot = %q, want test/self-improve/repo", got)
	}
}

func TestSelfImprovement_PatchAndSandboxNegativeFallsBack(t *testing.T) {
	c := &SelfImprovement{
		Patch: &SelfImprovement_Patch{MaxDiffLines: -1, DailyAutoApplyQuota: -1, MaxAttempts: -1},
		Sandbox: &SelfImprovement_Sandbox{
			GateTimeouts: &SelfImprovement_Sandbox_GateTimeouts{
				G1: durationpb.New(-time.Minute),
				G2: durationpb.New(0),
				G3: durationpb.New(-time.Second),
			},
		},
	}
	if got := c.SIMaxDiffLines(); got != 500 {
		t.Errorf("SIMaxDiffLines = %d, want 500（负值回退默认）", got)
	}
	if got := c.SIDailyAutoApplyQuota(); got != 5 {
		t.Errorf("SIDailyAutoApplyQuota = %d, want 5", got)
	}
	if got := c.SIMaxAttempts(); got != 3 {
		t.Errorf("SIMaxAttempts = %d, want 3", got)
	}
	gt := c.SIGateTimeouts()
	if gt["g1"] != 5*time.Minute || gt["g2"] != 10*time.Minute || gt["g3"] != 5*time.Minute {
		t.Errorf("SIGateTimeouts = %v, want defaults", gt)
	}
}

func TestSelfImprovement_NegativeFallsBackToDefault(t *testing.T) {
	c := &SelfImprovement{
		ObserveInterval: durationpb.New(-time.Minute),
		ErrorCluster:    &SelfImprovement_ErrorCluster{WindowDays: -1, MinCount: -2},
		Perf:            &SelfImprovement_Perf{LatencyFactor: -1, TokenFactor: -1},
		Eval:            &SelfImprovement_Eval{RegressionThreshold: -0.5},
	}
	if got := c.SIObserveInterval(); got != 15*time.Minute {
		t.Errorf("SIObserveInterval = %v, want 15m（负值回退默认）", got)
	}
	if got := c.SIErrorClusterWindowDays(); got != 7 {
		t.Errorf("SIErrorClusterWindowDays = %d, want 7", got)
	}
	if got := c.SIErrorClusterMinCount(); got != 5 {
		t.Errorf("SIErrorClusterMinCount = %d, want 5", got)
	}
	if got := c.SIPerfLatencyFactor(); got != 2.0 {
		t.Errorf("SIPerfLatencyFactor = %v, want 2.0", got)
	}
	if got := c.SIPerfTokenFactor(); got != 1.5 {
		t.Errorf("SIPerfTokenFactor = %v, want 1.5", got)
	}
	if got := c.SIEvalRegressionThreshold(); got != 0.10 {
		t.Errorf("SIEvalRegressionThreshold = %v, want 0.10", got)
	}
}

func TestSelfImprovement_ObserveWindowDefaults(t *testing.T) {
	var nilConf *SelfImprovement
	zero := &SelfImprovement{}
	for name, c := range map[string]*SelfImprovement{"nil": nilConf, "zero": zero} {
		if got := c.SIObserveWindowDuration(); got != 24*time.Hour {
			t.Errorf("%s: SIObserveWindowDuration = %v, want 24h", name, got)
		}
		if got := c.SIObserveErrorRateFactor(); got != 1.5 {
			t.Errorf("%s: SIObserveErrorRateFactor = %v, want 1.5", name, got)
		}
		if got := c.SIObserveP95Factor(); got != 1.3 {
			t.Errorf("%s: SIObserveP95Factor = %v, want 1.3", name, got)
		}
		if got := c.SIMaxConcurrentObserving(); got != 3 {
			t.Errorf("%s: SIMaxConcurrentObserving = %d, want 3", name, got)
		}
		if got := c.SIWatchdogInterval(); got != 5*time.Minute {
			t.Errorf("%s: SIWatchdogInterval = %v, want 5m", name, got)
		}
		if got := c.SIOutcomeInterval(); got != time.Hour {
			t.Errorf("%s: SIOutcomeInterval = %v, want 1h", name, got)
		}
	}
}

func TestSelfImprovement_ObserveWindowExplicitAndNegative(t *testing.T) {
	c := &SelfImprovement{
		ObserveWindow: &SelfImprovement_ObserveWindow{
			Duration:               durationpb.New(2 * time.Hour),
			ErrorRateFactor:        2.0,
			P95Factor:              1.1,
			MaxConcurrentObserving: 5,
		},
		WatchdogInterval: durationpb.New(time.Minute),
		OutcomeInterval:  durationpb.New(30 * time.Minute),
	}
	if got := c.SIObserveWindowDuration(); got != 2*time.Hour {
		t.Errorf("SIObserveWindowDuration = %v, want 2h", got)
	}
	if got := c.SIObserveErrorRateFactor(); got != 2.0 {
		t.Errorf("SIObserveErrorRateFactor = %v, want 2.0", got)
	}
	if got := c.SIObserveP95Factor(); got != 1.1 {
		t.Errorf("SIObserveP95Factor = %v, want 1.1", got)
	}
	if got := c.SIMaxConcurrentObserving(); got != 5 {
		t.Errorf("SIMaxConcurrentObserving = %d, want 5", got)
	}
	if got := c.SIWatchdogInterval(); got != time.Minute {
		t.Errorf("SIWatchdogInterval = %v, want 1m", got)
	}
	if got := c.SIOutcomeInterval(); got != 30*time.Minute {
		t.Errorf("SIOutcomeInterval = %v, want 30m", got)
	}

	neg := &SelfImprovement{
		ObserveWindow: &SelfImprovement_ObserveWindow{
			Duration:               durationpb.New(-time.Hour),
			ErrorRateFactor:        -1,
			P95Factor:              -1,
			MaxConcurrentObserving: -1,
		},
		WatchdogInterval: durationpb.New(-time.Minute),
		OutcomeInterval:  durationpb.New(0),
	}
	if got := neg.SIObserveWindowDuration(); got != 24*time.Hour {
		t.Errorf("neg: SIObserveWindowDuration = %v, want 24h", got)
	}
	if got := neg.SIObserveErrorRateFactor(); got != 1.5 {
		t.Errorf("neg: SIObserveErrorRateFactor = %v, want 1.5", got)
	}
	if got := neg.SIObserveP95Factor(); got != 1.3 {
		t.Errorf("neg: SIObserveP95Factor = %v, want 1.3", got)
	}
	if got := neg.SIMaxConcurrentObserving(); got != 3 {
		t.Errorf("neg: SIMaxConcurrentObserving = %d, want 3", got)
	}
	if got := neg.SIWatchdogInterval(); got != 5*time.Minute {
		t.Errorf("neg: SIWatchdogInterval = %v, want 5m", got)
	}
	if got := neg.SIOutcomeInterval(); got != time.Hour {
		t.Errorf("neg: SIOutcomeInterval = %v, want 1h", got)
	}
}
