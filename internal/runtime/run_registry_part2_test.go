package runtime

import (
	"aranea-agents/internal/biz"
	"testing"
	"time"
)

// TestRunRegistryFinish_StaleRunIDDoesNotDeleteNewEntry verifies C-09:
// Cancel → force Cancel → StoreRunner(new) → Finish(oldRunID) must not
// wipe the newer entry.
func TestRunRegistryFinish_StaleRunIDDoesNotDeleteNewEntry(t *testing.T) {
	reg := NewRunRegistry()
	oldRunner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-old", oldRunner)

	if stopped, id := reg.Cancel("session-1", ""); !stopped || id != "run-old" {
		t.Fatalf("first Cancel() = (%v, %q), want (true, run-old)", stopped, id)
	}
	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatal("force Cancel = false, want true")
	}
	if reg.HasActive("session-1") {
		t.Fatal("HasActive = true after force Cancel, want false")
	}

	newRunner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-new", newRunner)
	if !reg.HasActive("session-1") {
		t.Fatal("HasActive = false after StoreRunner(new), want true")
	}

	reg.Finish("session-1", "run-old")
	if !reg.HasActive("session-1") {
		t.Fatal("HasActive = false after stale Finish(run-old), want true (run-new preserved)")
	}
	runner, runID, ok := reg.ActiveRunner("session-1")
	if !ok || runID != "run-new" || runner != newRunner {
		t.Fatalf("ActiveRunner() = (%v, %q, %v), want (newRunner, run-new, true)", runner, runID, ok)
	}

	reg.Finish("session-1", "run-new")
	if reg.HasActive("session-1") {
		t.Fatal("HasActive = true after Finish(run-new), want false")
	}
}

// TestRunRegistryFinish_RetainsTerminalStatus verifies R4-Q2: Finish keeps a
// terminal status queryable for TerminalStatusTTL so run-status polling sees
// the terminal state instead of 404; non-terminal leftovers are dropped, and
// expired terminal entries are lazily removed by GetStatus.
func TestRunRegistryFinish_RetainsTerminalStatus(t *testing.T) {
	reg := NewRunRegistry()
	reg.StoreRunner("session-1", "run-1", &registryRunner{cancelOK: true})
	reg.SetStatus("session-1", "run-1", biz.SessionRunPhaseCompleted, "")

	reg.Finish("session-1", "run-1")
	if reg.HasActive("session-1") {
		t.Fatal("HasActive = true after Finish, want false")
	}
	status, ok := reg.GetStatus("session-1")
	if !ok || status.Status != biz.SessionRunPhaseCompleted || status.RunID != "run-1" {
		t.Fatalf("GetStatus() = (%+v, %v), want retained completed run-1", status, ok)
	}

	reg.SetStatus("session-1", "run-1", biz.SessionRunPhaseCompleted, "")
	reg.runStatuses.store("session-1", &RunStatusEntry{
		RunID: "run-1", Status: biz.SessionRunPhaseCompleted,
		UpdatedAt: time.Now().Add(-TerminalStatusTTL - time.Minute),
	})
	if _, ok := reg.GetStatus("session-1"); ok {
		t.Fatal("GetStatus() = true for expired terminal entry, want false")
	}

	reg2 := NewRunRegistry()
	reg2.StoreRunner("session-2", "run-2", &registryRunner{cancelOK: true})
	reg2.SetStatus("session-2", "run-2", RunStatusRunning, "")
	reg2.Finish("session-2", "run-2")
	if _, ok := reg2.GetStatus("session-2"); ok {
		t.Fatal("GetStatus() = true for non-terminal entry after Finish, want false")
	}
}
