package runtime

import "testing"

// E2E-B1: PauseSession cancels the active run and then re-marks the session
// "paused" (chat_pause.go publishPauseResumeStatus). The cancelled runner's
// deferred Finish used to delete the fresh paused entry (paused is
// non-terminal), so ResumeSession's GetRunStatus gate could never observe a
// resumable state and conflicted (409) forever.
//
// Finish must now keep (a) terminal entries for the polling window, (b) paused
// entries as lifecycle holds, and (c) entries tagged with a different (newer)
// runID — dropping only abnormal non-terminal entries of the finishing run.

func TestRunRegistryFinishKeepsPausedStatus(t *testing.T) {
	reg := NewRunRegistry()
	reg.SetStatus("s1", "r1", RunStatusPaused, "")

	reg.Finish("s1", "r1")

	st, ok := reg.GetStatus("s1")
	if !ok {
		t.Fatal("paused status wiped by cancelled runner's Finish; resume would 409 forever")
	}
	if st.Status != RunStatusPaused || st.RunID != "r1" {
		t.Fatalf("status = %+v, want paused r1", st)
	}
}

func TestRunRegistryResumeGateSeesPausedAfterTeardown(t *testing.T) {
	// Full B1 sequence: run starts, pause cancels + re-marks paused, the
	// runner's deferred Finish fires, then the resume gate polls GetStatus.
	reg := NewRunRegistry()
	reg.SetStatus("s1", "r1", RunStatusRunning, "")
	reg.SetStatus("s1", "r1", RunStatusPaused, "") // publishPauseResumeStatus
	reg.Finish("s1", "r1")                         // cancelled runner teardown defer

	st, ok := reg.GetStatus("s1")
	if !ok || st.Status != RunStatusPaused {
		t.Fatalf("resume gate would miss paused state: %+v ok=%v", st, ok)
	}
}

func TestRunRegistryFinishDropsAbnormalNonTerminalStatus(t *testing.T) {
	reg := NewRunRegistry()
	reg.SetStatus("s1", "r1", RunStatusRunning, "")

	reg.Finish("s1", "r1")

	if _, ok := reg.GetStatus("s1"); ok {
		t.Fatal("non-terminal non-paused entry of the finishing run must be dropped (abnormal path)")
	}
}

func TestRunRegistryFinishKeepsTerminalStatus(t *testing.T) {
	reg := NewRunRegistry()
	reg.SetStatus("s1", "r1", RunStatusCompleted, "")

	reg.Finish("s1", "r1")

	st, ok := reg.GetStatus("s1")
	if !ok || st.Status != RunStatusCompleted {
		t.Fatalf("terminal status = %+v ok=%v, want kept for polling window", st, ok)
	}
}

func TestRunRegistryFinishKeepsNewerRunStatus(t *testing.T) {
	reg := NewRunRegistry()
	// A newer run already re-marked the session; the stale runner's Finish
	// must not touch its entry.
	reg.SetStatus("s1", "r2", RunStatusRunning, "")

	reg.Finish("s1", "r1")

	st, ok := reg.GetStatus("s1")
	if !ok || st.RunID != "r2" {
		t.Fatalf("newer run's status = %+v ok=%v, want kept", st, ok)
	}
}
