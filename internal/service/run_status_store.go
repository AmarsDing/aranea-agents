package service

import (
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// persistedRunStatus is the on-disk shape stored in session metadata for run lifecycle.
type persistedRunStatus struct {
	RunID           string
	Status          string
	ErrorMessage    string
	UpdatedAt       string
	AwaitKind       string
	AwaitToolKey    string
	AwaitToolCallID string
}

const (
	stateKeyRunID           = "runtime.run_id"
	stateKeyRunStatus       = "runtime.status"
	stateKeyRunError        = "runtime.error_message"
	stateKeyRunUpdatedAt    = "runtime.updated_at"
	stateKeyAwaitRunID      = "runtime.await_run_id"
	stateKeyAwaitSince      = "runtime.await_since"
	stateKeyAwaitKind       = "runtime.await_kind"
	stateKeyAwaitToolKey    = "runtime.await_tool_key"
	stateKeyAwaitToolCallID = "runtime.await_tool_call_id"
)

func terminalRunStatus(status string) bool {
	return biz.IsSessionRunPhaseTerminal(biz.ParseSessionRunPhase(status)) || strings.TrimSpace(strings.ToLower(status)) == "idle"
}

// runStatusSnapshotFresh reports whether a persisted run-status snapshot's
// updatedAt is within ttl of now. Unparseable/empty timestamps count as stale
// (R4-Q2: fresh terminal snapshots are returned 200; stale ones 404).
func runStatusSnapshotFresh(updatedAt string, ttl time.Duration) bool {
	ts := strings.TrimSpace(updatedAt)
	if ts == "" || ttl <= 0 {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	return time.Since(parsed) <= ttl
}
