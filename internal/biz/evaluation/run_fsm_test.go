package evaluation

import "testing"

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
	}{
		{"", RunStatusPending, true},
		{RunStatusPending, RunStatusPending, true},
		{RunStatusPending, RunStatusRunning, true},
		{RunStatusPending, RunStatusCancelled, true},
		{RunStatusPending, RunStatusFailed, true},
		{RunStatusPending, RunStatusCompleted, false},
		{RunStatusRunning, RunStatusCompleted, true},
		{RunStatusRunning, RunStatusFailed, true},
		{RunStatusRunning, RunStatusCancelled, true},
		{RunStatusRunning, RunStatusPending, false},
		{RunStatusCompleted, RunStatusFailed, false},
		{RunStatusFailed, RunStatusRunning, false},
		{RunStatusCancelled, RunStatusCompleted, false},
		{"bogus", RunStatusRunning, false},
	}
	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.ok {
			t.Errorf("%q → %q = %v, want %v", tc.from, tc.to, got, tc.ok)
		}
	}
}

func TestValidateTransitionRejectsIllegal(t *testing.T) {
	if err := ValidateTransition(RunStatusCompleted, RunStatusRunning); err == nil {
		t.Fatal("completed → running must fail")
	}
	if err := ValidateTransition(RunStatusRunning, RunStatusCompleted); err != nil {
		t.Fatalf("running → completed: %v", err)
	}
}
