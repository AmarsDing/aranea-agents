package service

import (
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/pkg/loggateway"
)

func TestPlanExecutor_TrackReleaseTeamAgentLoads(t *testing.T) {
	prev := chatagent.GlobalAgentLoadTracker
	chatagent.GlobalAgentLoadTracker = chatagent.NewAgentLoadTracker()
	t.Cleanup(func() { chatagent.GlobalAgentLoadTracker = prev })

	pe := NewPlanExecutor(nil, nil, nil, loggateway.NewNoop())
	pe.trackTeamAgentLoads("team-1", []string{"a", "a", "b", ""})
	if chatagent.GlobalAgentLoadTracker.Load("a") != 1 || chatagent.GlobalAgentLoadTracker.Load("b") != 1 {
		t.Fatalf("loads a=%d b=%d", chatagent.GlobalAgentLoadTracker.Load("a"), chatagent.GlobalAgentLoadTracker.Load("b"))
	}
	// Idempotent
	pe.trackTeamAgentLoads("team-1", []string{"a", "b"})
	if chatagent.GlobalAgentLoadTracker.Load("a") != 1 {
		t.Fatal("double acquire")
	}
	pe.NotifyTeamCompletion("team-1", "run-1", true, "")
	if chatagent.GlobalAgentLoadTracker.Load("a") != 0 || chatagent.GlobalAgentLoadTracker.Load("b") != 0 {
		t.Fatalf("after release a=%d b=%d", chatagent.GlobalAgentLoadTracker.Load("a"), chatagent.GlobalAgentLoadTracker.Load("b"))
	}
}