package conf

import "time"

// SelfImprovement config accessors (73-self-iteration-v3, design §6.2).
// All accessors are nil-safe; zero/negative values fall back to the design
// defaults so an empty `self_improvement:` block equals "disabled with
// defaults".

const (
	siDefaultObserveInterval        = 15 * time.Minute
	siDefaultErrorClusterWindow     = 7
	siDefaultErrorClusterMinCount   = 5
	siDefaultPerfLatencyFactor      = 2.0
	siDefaultPerfTokenFactor        = 1.5
	siDefaultEvalThreshold          = 0.10
	siDefaultMaxDiffLines           = 500
	siDefaultDailyAutoApplyQuota    = 5
	siDefaultMaxAttempts            = 3
	siDefaultGateG1Timeout          = 5 * time.Minute
	siDefaultGateG2Timeout          = 10 * time.Minute
	siDefaultGateG3Timeout          = 5 * time.Minute
	siDefaultWorktreeRoot           = ".aranea-self-improve"
	siDefaultObserveWindowDuration  = 24 * time.Hour
	siDefaultObserveErrorRateFactor = 1.5
	siDefaultObserveP95Factor       = 1.3
	siDefaultMaxConcurrentObserving = 3
	siDefaultWatchdogInterval       = 5 * time.Minute
	siDefaultOutcomeInterval        = time.Hour
	siDefaultDriveInterval          = time.Minute
	siDefaultStaleTimeout           = 30 * time.Minute
)

// SIEnabled reports whether the platform self-improvement pipeline is on.
// Default false (gray rollout).
func (c *SelfImprovement) SIEnabled() bool {
	return c.GetEnabled()
}

// SIObserveInterval is the observe-worker scan interval (default 15m).
func (c *SelfImprovement) SIObserveInterval() time.Duration {
	if d := c.GetObserveInterval().AsDuration(); d > 0 {
		return d
	}
	return siDefaultObserveInterval
}

// SIErrorClusterWindowDays is the error-cluster observation window in days
// (default 7).
func (c *SelfImprovement) SIErrorClusterWindowDays() int {
	if v := int(c.GetErrorCluster().GetWindowDays()); v > 0 {
		return v
	}
	return siDefaultErrorClusterWindow
}

// SIErrorClusterMinCount is the min occurrences in the window to fire
// (default 5).
func (c *SelfImprovement) SIErrorClusterMinCount() int {
	if v := int(c.GetErrorCluster().GetMinCount()); v > 0 {
		return v
	}
	return siDefaultErrorClusterMinCount
}

// SIPerfLatencyFactor is the p95 regression factor vs baseline (default 2.0).
func (c *SelfImprovement) SIPerfLatencyFactor() float64 {
	if v := c.GetPerf().GetLatencyFactor(); v > 0 {
		return v
	}
	return siDefaultPerfLatencyFactor
}

// SIPerfTokenFactor is the token-usage anomaly factor (default 1.5).
func (c *SelfImprovement) SIPerfTokenFactor() float64 {
	if v := c.GetPerf().GetTokenFactor(); v > 0 {
		return v
	}
	return siDefaultPerfTokenFactor
}

// SIEvalRegressionThreshold is the eval score drop ratio to fire
// (default 0.10).
func (c *SelfImprovement) SIEvalRegressionThreshold() float64 {
	if v := c.GetEval().GetRegressionThreshold(); v > 0 {
		return v
	}
	return siDefaultEvalThreshold
}

// SITestRunsDir is the directory of test-run JSON round files feeding the
// test_failure trigger. Empty = that signal source stays inert.
func (c *SelfImprovement) SITestRunsDir() string {
	return c.GetTestRunsDir()
}

// ── Patch governance (design §6.2 / D10) ────────────────────────────────────

// SIMaxDiffLines is the diff size cap in changed lines (default 500).
func (c *SelfImprovement) SIMaxDiffLines() int {
	if v := int(c.GetPatch().GetMaxDiffLines()); v > 0 {
		return v
	}
	return siDefaultMaxDiffLines
}

// SIDailyAutoApplyQuota is the daily auto-apply quota (default 5).
func (c *SelfImprovement) SIDailyAutoApplyQuota() int {
	if v := int(c.GetPatch().GetDailyAutoApplyQuota()); v > 0 {
		return v
	}
	return siDefaultDailyAutoApplyQuota
}

// SIMaxAttempts is the patch-verify retry cap (default 3).
func (c *SelfImprovement) SIMaxAttempts() int {
	if v := int(c.GetPatch().GetMaxAttempts()); v > 0 {
		return v
	}
	return siDefaultMaxAttempts
}

// ── Sandbox (design §6.2 / D4) ──────────────────────────────────────────────

// SIGateTimeouts returns the per-gate execution timeouts keyed by gate suffix
// ("g1"/"g2"/"g3"); defaults 5m/10m/5m (design D4).
func (c *SelfImprovement) SIGateTimeouts() map[string]time.Duration {
	gt := c.GetSandbox().GetGateTimeouts()
	out := map[string]time.Duration{
		"g1": siDefaultGateG1Timeout,
		"g2": siDefaultGateG2Timeout,
		"g3": siDefaultGateG3Timeout,
	}
	if d := gt.GetG1().AsDuration(); d > 0 {
		out["g1"] = d
	}
	if d := gt.GetG2().AsDuration(); d > 0 {
		out["g2"] = d
	}
	if d := gt.GetG3().AsDuration(); d > 0 {
		out["g3"] = d
	}
	return out
}

// SIWorktreeRoot is the sandbox worktree root relative to repo root
// (default ".aranea-self-improve").
func (c *SelfImprovement) SIWorktreeRoot() string {
	if v := c.GetSandbox().GetWorktreeRoot(); v != "" {
		return v
	}
	return siDefaultWorktreeRoot
}

// SIRepoRoot is the platform repository root the sandbox/applier operates
// on. Empty = the provider falls back to the process working directory.
func (c *SelfImprovement) SIRepoRoot() string {
	return c.GetSandbox().GetRepoRoot()
}

// ── Observe window (design §6.2 / D7 / D10) ─────────────────────────────────

// SIObserveWindowDuration is the post-apply observing window length
// (default 24h).
func (c *SelfImprovement) SIObserveWindowDuration() time.Duration {
	if d := c.GetObserveWindow().GetDuration().AsDuration(); d > 0 {
		return d
	}
	return siDefaultObserveWindowDuration
}

// SIObserveErrorRateFactor is the rollback threshold: after.error_rate >
// before.error_rate*factor triggers auto-rollback (default 1.5).
func (c *SelfImprovement) SIObserveErrorRateFactor() float64 {
	if v := c.GetObserveWindow().GetErrorRateFactor(); v > 0 {
		return v
	}
	return siDefaultObserveErrorRateFactor
}

// SIObserveP95Factor is the rollback threshold: after.p95 > before.p95*factor
// triggers auto-rollback (default 1.3).
func (c *SelfImprovement) SIObserveP95Factor() float64 {
	if v := c.GetObserveWindow().GetP95Factor(); v > 0 {
		return v
	}
	return siDefaultObserveP95Factor
}

// SIMaxConcurrentObserving caps concurrent observing runs (default 3, D10).
func (c *SelfImprovement) SIMaxConcurrentObserving() int {
	if v := int(c.GetObserveWindow().GetMaxConcurrentObserving()); v > 0 {
		return v
	}
	return siDefaultMaxConcurrentObserving
}

// SIWatchdogInterval is the watchdog worker tick (default 5m, design §5).
func (c *SelfImprovement) SIWatchdogInterval() time.Duration {
	if d := c.GetWatchdogInterval().AsDuration(); d > 0 {
		return d
	}
	return siDefaultWatchdogInterval
}

// SIOutcomeInterval is the outcome attribution worker tick (default 1h,
// design §5).
func (c *SelfImprovement) SIOutcomeInterval() time.Duration {
	if d := c.GetOutcomeInterval().AsDuration(); d > 0 {
		return d
	}
	return siDefaultOutcomeInterval
}

// SIDriveInterval is the full-chain drive worker tick (default 1m).
func (c *SelfImprovement) SIDriveInterval() time.Duration {
	if d := c.GetDriveInterval().AsDuration(); d > 0 {
		return d
	}
	return siDefaultDriveInterval
}

// SIStaleTimeout is the mid-pipeline stale threshold: runs idle longer than
// this are recovered to detected for re-driving (default 30m).
func (c *SelfImprovement) SIStaleTimeout() time.Duration {
	if d := c.GetStaleTimeout().AsDuration(); d > 0 {
		return d
	}
	return siDefaultStaleTimeout
}
