package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestIsTeamRunTerminalStatus(t *testing.T) {
	terminal := []string{TeamRunStatusSuccess, TeamRunStatusFailed, TeamRunStatusCancelled}
	for _, s := range terminal {
		if !IsTeamRunTerminalStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	nonTerminal := []string{TeamRunStatusPending, TeamRunStatusRunning, TeamRunStatusWaitingHuman}
	for _, s := range nonTerminal {
		if IsTeamRunTerminalStatus(s) {
			t.Errorf("expected %q to NOT be terminal", s)
		}
	}
}

func TestValidateTeamRunTransition(t *testing.T) {
	sm := NewTeamRunStateMachine()
	valid := [][2]string{
		{TeamRunStatusPending, TeamRunStatusRunning},
		{TeamRunStatusPending, TeamRunStatusCancelled},
		{TeamRunStatusRunning, TeamRunStatusWaitingHuman},
		{TeamRunStatusRunning, TeamRunStatusSuccess},
		{TeamRunStatusRunning, TeamRunStatusFailed},
		{TeamRunStatusRunning, TeamRunStatusCancelled},
		{TeamRunStatusWaitingHuman, TeamRunStatusRunning},
		{TeamRunStatusWaitingHuman, TeamRunStatusSuccess},
		{TeamRunStatusWaitingHuman, TeamRunStatusFailed},
		{TeamRunStatusWaitingHuman, TeamRunStatusCancelled},
	}
	for _, pair := range valid {
		if !sm.CanTransition(TeamRunState(pair[0]), TeamRunState(pair[1])) {
			t.Errorf("expected transition %s → %s to be valid", pair[0], pair[1])
		}
	}
	invalid := [][2]string{
		{TeamRunStatusPending, TeamRunStatusSuccess},
		{TeamRunStatusPending, TeamRunStatusFailed},
		{TeamRunStatusPending, TeamRunStatusWaitingHuman},
		{TeamRunStatusSuccess, TeamRunStatusRunning},
		{TeamRunStatusFailed, TeamRunStatusRunning},
		{TeamRunStatusCancelled, TeamRunStatusRunning},
		{TeamRunStatusSuccess, TeamRunStatusFailed},
		{TeamRunStatusFailed, TeamRunStatusSuccess},
	}
	for _, pair := range invalid {
		if sm.CanTransition(TeamRunState(pair[0]), TeamRunState(pair[1])) {
			t.Errorf("expected transition %s → %s to be INVALID", pair[0], pair[1])
		}
	}
}

func TestParseOrchestrationDecision(t *testing.T) {
	d, err := ParseOrchestrationDecision([]byte(`{"action":"approve","score":0.9,"reason":"good"}`), loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != "approve" || d.Score != 0.9 || d.Reason != "good" {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestIsApprovedDecision(t *testing.T) {
	if !IsApprovedDecision(OrchestrationDecision{Action: "approve"}, 0) {
		t.Fatal("approve action should be approved")
	}
	if IsApprovedDecision(OrchestrationDecision{Action: "retry"}, 0) {
		t.Fatal("retry action should not be approved with threshold 0")
	}
	// 显式 retry 不得被 score 推翻。
	if IsApprovedDecision(OrchestrationDecision{Action: "retry", Score: 0.9}, 0.8) {
		t.Fatal("explicit retry must not be overridden by score")
	}
	// action 为空时 score 才兜底。
	if !IsApprovedDecision(OrchestrationDecision{Score: 0.9}, 0.8) {
		t.Fatal("empty action falls back to score >= threshold")
	}
	if IsApprovedDecision(OrchestrationDecision{Score: 0.5}, 0.8) {
		t.Fatal("empty action with score below threshold should not be approved")
	}
	if !IsApprovedDecision(OrchestrationDecision{Action: "Approve"}, 0) {
		t.Fatal("action matching should be case-insensitive")
	}
}

func TestExtractScore(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{`{"score": 0.85}`, 0.85},
		{`[{"score": 0.9}]`, 0.9},
		{`no score here`, 0},
		{``, 0},
	}
	for _, tt := range tests {
		got := ExtractScore(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractScore(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

// --- T1.2: Team status state machine tests ---

func TestValidTeamStatusTransition(t *testing.T) {
	sm := NewTeamStateMachine()
	valid := [][2]string{
		// pending → running, cancelled, failed
		{TeamStatusPending, TeamStatusRunning},
		{TeamStatusPending, TeamStatusCancelled},
		{TeamStatusPending, TeamStatusFailed},
		// running → completed, failed, cancelled, interrupted, pending (rework)
		{TeamStatusRunning, TeamStatusCompleted},
		{TeamStatusRunning, TeamStatusFailed},
		{TeamStatusRunning, TeamStatusCancelled},
		{TeamStatusRunning, TeamStatusInterrupted},
		{TeamStatusRunning, TeamStatusPending},
		// interrupted → running (recovery)
		{TeamStatusInterrupted, TeamStatusRunning},
		// failed/cancelled → pending (recover, used by RetryTeam)
		{TeamStatusFailed, TeamStatusPending},
		{TeamStatusCancelled, TeamStatusPending},
	}
	for _, pair := range valid {
		if !sm.CanTransition(TeamState(pair[0]), TeamState(pair[1])) {
			t.Errorf("expected team transition %s → %s to be valid", pair[0], pair[1])
		}
	}

	invalid := [][2]string{
		// Terminal states cannot transition out (archived is the only true terminal)
		{TeamStatusCompleted, TeamStatusRunning},
		{TeamStatusCompleted, TeamStatusPending},
		{TeamStatusFailed, TeamStatusRunning},
		{TeamStatusCancelled, TeamStatusRunning},
		// Cannot skip states
		{TeamStatusPending, TeamStatusCompleted},
		{TeamStatusPending, TeamStatusInterrupted},
		// interrupted can only go to running
		{TeamStatusInterrupted, TeamStatusCompleted},
		{TeamStatusInterrupted, TeamStatusFailed},
		{TeamStatusInterrupted, TeamStatusCancelled},
		{TeamStatusInterrupted, TeamStatusPending},
	}
	for _, pair := range invalid {
		if sm.CanTransition(TeamState(pair[0]), TeamState(pair[1])) {
			t.Errorf("expected team transition %s → %s to be INVALID", pair[0], pair[1])
		}
	}
}

func TestIsTeamStatusActive(t *testing.T) {
	active := []string{TeamStatusPending, TeamStatusRunning, TeamStatusInterrupted}
	for _, s := range active {
		if !IsTeamStatusActive(s) {
			t.Errorf("expected %q to be active", s)
		}
	}
	inactive := []string{TeamStatusCompleted, TeamStatusFailed, TeamStatusCancelled, TeamStatusArchived}
	for _, s := range inactive {
		if IsTeamStatusActive(s) {
			t.Errorf("expected %q to NOT be active", s)
		}
	}
}

func TestTeamStatusConstants_NoOldStates(t *testing.T) {
	// Verify that old status values are not present as constants.
	// The state machine should only have: pending, running, completed, failed, cancelled, interrupted.
	allStatuses := map[string]bool{
		TeamStatusPending:     true,
		TeamStatusRunning:     true,
		TeamStatusCompleted:   true,
		TeamStatusFailed:      true,
		TeamStatusCancelled:   true,
		TeamStatusInterrupted: true,
	}
	for _, old := range []string{"active", "assembled", "waiting_deps"} {
		if allStatuses[old] {
			t.Errorf("old status %q should not be in the state machine", old)
		}
	}
	if len(allStatuses) != 6 {
		t.Errorf("expected exactly 6 team statuses, got %d", len(allStatuses))
	}
}
