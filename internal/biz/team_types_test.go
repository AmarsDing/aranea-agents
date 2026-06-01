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
		{TeamRunStatusRunning, TeamRunStatusRunning},
		{TeamRunStatusPending, TeamRunStatusPending},
	}
	for _, pair := range valid {
		if !ValidateTeamRunTransition(pair[0], pair[1]) {
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
		if ValidateTeamRunTransition(pair[0], pair[1]) {
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
	if !IsApprovedDecision(OrchestrationDecision{Action: "retry", Score: 0.9}, 0.8) {
		t.Fatal("score above threshold should be approved")
	}
	if IsApprovedDecision(OrchestrationDecision{Action: "retry", Score: 0.5}, 0.8) {
		t.Fatal("score below threshold should not be approved")
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
