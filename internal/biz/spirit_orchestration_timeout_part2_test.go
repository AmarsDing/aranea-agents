package biz

import (
	"context"
	"testing"
	"time"
)

// 降级路径：stepReader 不具备活动探测能力时退化为 run 起点活动
// （等价旧 wall-clock 语义），仍受 maxLifetime 兜底。
func TestTeamTimeout_NoActivityProbeDegradesToRunStart(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-degraded", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-degraded",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		}},
	}

	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, nil)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-degraded")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-degraded")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-degraded")
		t.Fatalf("degraded team status=%q, want failed (run-start activity only)", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}
