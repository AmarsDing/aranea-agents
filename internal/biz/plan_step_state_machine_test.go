package biz

import (
	"strings"
	"testing"
)

func TestPlanStep_Transition_Valid(t *testing.T) {
	cases := []struct {
		name string
		from PlanStepStatus
		to   PlanStepStatus
	}{
		{"pending to running", PlanStepStatusPending, PlanStepStatusRunning},
		{"pending to skipped", PlanStepStatusPending, PlanStepStatusSkipped},
		{"running to completed", PlanStepStatusRunning, PlanStepStatusCompleted},
		{"running to failed", PlanStepStatusRunning, PlanStepStatusFailed},
		{"running to partial_failure", PlanStepStatusRunning, PlanStepStatusPartialFailure},
		{"failed to running (retry)", PlanStepStatusFailed, PlanStepStatusRunning},
		{"partial_failure to running (retry)", PlanStepStatusPartialFailure, PlanStepStatusRunning},
		// F2（2026-09-03）：resume 恢复边。
		{"failed to pending (resume retry)", PlanStepStatusFailed, PlanStepStatusPending},
		{"failed to skipped (resume skip)", PlanStepStatusFailed, PlanStepStatusSkipped},
		{"skipped to pending (resume revive)", PlanStepStatusSkipped, PlanStepStatusPending},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := PlanStep{Status: c.from}
			if err := ps.Transition(c.to); err != nil {
				t.Fatalf("expected transition %s → %s to succeed, got: %v", c.from, c.to, err)
			}
			if ps.Status != c.to {
				t.Fatalf("expected status %s, got %s", c.to, ps.Status)
			}
		})
	}
}

func TestPlanStep_Transition_Invalid(t *testing.T) {
	cases := []struct {
		name string
		from PlanStepStatus
		to   PlanStepStatus
	}{
		{"completed to running (terminal)", PlanStepStatusCompleted, PlanStepStatusRunning},
		{"skipped to running (terminal)", PlanStepStatusSkipped, PlanStepStatusRunning},
		{"completed to failed (terminal)", PlanStepStatusCompleted, PlanStepStatusFailed},
		{"pending to completed (skip running)", PlanStepStatusPending, PlanStepStatusCompleted},
		{"pending to failed (skip running)", PlanStepStatusPending, PlanStepStatusFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := PlanStep{Status: c.from}
			err := ps.Transition(c.to)
			if err == nil {
				t.Fatalf("expected transition %s → %s to fail, but succeeded", c.from, c.to)
			}
			if !strings.Contains(err.Error(), "invalid transition") {
				t.Fatalf("expected 'invalid transition' in error, got: %v", err)
			}
			if ps.Status != c.from {
				t.Fatalf("status should remain %s, got %s", c.from, ps.Status)
			}
		})
	}
}

func TestPlanStep_Transition_UnknownSource(t *testing.T) {
	ps := PlanStep{Status: PlanStepStatus("unknown")}
	err := ps.Transition(PlanStepStatusRunning)
	if err == nil {
		t.Fatalf("expected error for unknown source status")
	}
	if !strings.Contains(err.Error(), "unknown source status") {
		t.Fatalf("expected 'unknown source status' in error, got: %v", err)
	}
}

func TestPlanStep_CanTransition(t *testing.T) {
	ps := PlanStep{Status: PlanStepStatusPending}
	if !ps.CanTransition(PlanStepStatusRunning) {
		t.Fatalf("expected pending → running to be allowed")
	}
	if ps.CanTransition(PlanStepStatusCompleted) {
		t.Fatalf("expected pending → completed to be disallowed")
	}
}
