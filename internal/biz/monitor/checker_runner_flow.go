package monitor

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz/types"

	"github.com/google/uuid"
)

// defaultRunnerCompletionFlowWindow is the correlation window used by
// RunnerCompletionFlowChecker. It must be larger than the self-check
// interval (5 minutes) so a single missed cycle does not flap, and small
// enough that a broken recording pipeline is surfaced promptly.
const defaultRunnerCompletionFlowWindow = 30 * time.Minute

// RunnerCompletionFlowChecker verifies that the runner.completion recording
// pipeline is not silently broken.
//
// Historical incident: the RecordRunnerCompletion call was removed from the
// chat/team turn path, so no runner.completion events were persisted for 45
// days while chat traffic continued. No existing checker noticed because the
// alert eval worker was Ready and the trace projector kept receiving flow
// logs.
//
// The checker correlates two signals over a sliding window:
//   - flow activity: the trace projector received envelopes recently
//     (proof that chat/team runs are happening), and
//   - completion records: runner.completion rows persisted in the window.
//
// Flow activity without any completion record means the recording path is
// broken (Failed). No flow activity means the system is simply idle, which
// is healthy (Passed) and must not alert on low-traffic deployments.
type RunnerCompletionFlowChecker struct {
	repo      RunnerCompletionRepo
	projector TraceProjectorHealthChecker
	window    time.Duration
}

// NewRunnerCompletionFlowChecker creates the checker. A zero window falls
// back to the default (30 minutes).
func NewRunnerCompletionFlowChecker(repo RunnerCompletionRepo, projector TraceProjectorHealthChecker) *RunnerCompletionFlowChecker {
	return &RunnerCompletionFlowChecker{repo: repo, projector: projector, window: defaultRunnerCompletionFlowWindow}
}

func (c *RunnerCompletionFlowChecker) Name() string { return "runner_completion_flow" }

func (c *RunnerCompletionFlowChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.repo == nil || c.projector == nil {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "runner completion flow checker not fully wired (nil dependency)"
		return result
	}

	window := c.window
	if window <= 0 {
		window = defaultRunnerCompletionFlowWindow
	}

	recentFlow := c.projector.HasEverProcessed() && !c.projector.LastEventAt().IsZero() &&
		now.Sub(c.projector.LastEventAt()) <= window

	result.Details = map[string]any{
		"window_sec":        int(window.Seconds()),
		"recent_flow":       recentFlow,
		"last_flow_event_at": c.projector.LastEventAt(),
	}

	rows, err := c.repo.ListRecentRunnerCompletions(ctx, window, 1)
	if err != nil {
		// Transient DB issues must not page as a pipeline break; db_health
		// covers hard DB failures.
		result.Status = types.SelfCheckStatusWarning
		result.Message = "failed to query recent runner.completion records"
		result.Details["error"] = err.Error()
		return result
	}

	if len(rows) > 0 {
		result.Status = types.SelfCheckStatusPassed
		result.Message = "runner.completion stream healthy"
		result.Details["last_completion_at"] = rows[0].CreatedAt
		return result
	}

	if recentFlow {
		result.Status = types.SelfCheckStatusFailed
		result.Message = fmt.Sprintf(
			"runner.completion stream stalled: flow activity in the last %s but no completion records",
			window.Truncate(time.Second),
		)
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = "runner.completion stream idle (no flow activity in window)"
	return result
}
