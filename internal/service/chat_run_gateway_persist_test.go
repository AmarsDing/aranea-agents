package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/runtime"
)

// F9：biz.ChatPendingQueueCap（G1 满员拒绝阈值）必须与
// runtime.MaxPendingPerSession（真实队列容量）相等——前者大于后者时
// 「满员拒绝」永不触发（Enqueue 先容量拒绝），小于后者时队列尾部容量闲置。
// 两常量分属 biz/runtime 层无法互引，用适配层测试钉住绑定。
func TestChatPendingQueueCapMatchesRuntime(t *testing.T) {
	if biz.ChatPendingQueueCap != runtime.MaxPendingPerSession {
		t.Fatalf("ChatPendingQueueCap=%d must equal runtime.MaxPendingPerSession=%d",
			biz.ChatPendingQueueCap, runtime.MaxPendingPerSession)
	}
}

// recordingSessionStatePort is a test double for biz.SessionStatePort that
// records all PatchSessionState calls for verification.
type recordingSessionStatePort struct {
	mu      sync.Mutex
	state   map[string]string
	patches []recordedPatch
	err     error
}

type recordedPatch struct {
	sets    map[string]string
	deletes []string
}

func newRecordingSessionStatePort() *recordingSessionStatePort {
	return &recordingSessionStatePort{
		state: make(map[string]string),
	}
}

func (r *recordingSessionStatePort) GetSessionState(_ context.Context, _ string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]string, len(r.state))
	for k, v := range r.state {
		cp[k] = v
	}
	return cp, nil
}

func (r *recordingSessionStatePort) SaveSessionState(_ context.Context, _ string, state map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = make(map[string]string, len(state))
	for k, v := range state {
		r.state[k] = v
	}
	return nil
}

func (r *recordingSessionStatePort) PatchSessionState(_ context.Context, _ string, sets map[string]string, deletes []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patches = append(r.patches, recordedPatch{
		sets:    sets,
		deletes: deletes,
	})
	for k, v := range sets {
		r.state[k] = v
	}
	for _, k := range deletes {
		delete(r.state, k)
	}
	if r.err != nil {
		return r.err
	}
	return nil
}

func (r *recordingSessionStatePort) lastPatch() recordedPatch {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.patches) == 0 {
		return recordedPatch{}
	}
	return r.patches[len(r.patches)-1]
}

func (r *recordingSessionStatePort) GetSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (r *recordingSessionStatePort) BumpSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (r *recordingSessionStatePort) TransitionStatus(_ context.Context, _ string, _ session.SessionStatus, _ session.SessionStatusReason) error {
	return nil
}

// TestPersistRunStatusToSession_NonTerminalStatus_PersistsAllFields verifies
// that persistRunStatusToSession persists the full run status (run_id, status,
// updated_at, error_message) for non-terminal statuses like "running".
//
// B-06 root cause: the old implementation only persisted stateKeyRunID for
// non-terminal statuses, omitting stateKeyRunStatus, stateKeyRunError, and
// stateKeyRunUpdatedAt. This made HydrateRunStatusFromSession always return
// false (since it checks stateKeyRunStatus), breaking crash recovery.
func TestPersistRunStatusToSession_NonTerminalStatus_PersistsAllFields(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	ctx := context.Background()

	err := persistRunStatusToSession(sessions, ctx, "sess-1", "run-123", "running", "")
	if err != nil {
		t.Fatalf("persistRunStatusToSession failed: %v", err)
	}

	patch := sessions.lastPatch()
	if patch.sets[stateKeyRunID] != "run-123" {
		t.Errorf("stateKeyRunID=%q want %q", patch.sets[stateKeyRunID], "run-123")
	}
	if patch.sets[stateKeyRunStatus] != "running" {
		t.Errorf("stateKeyRunStatus=%q want %q (B-06: status not persisted)",
			patch.sets[stateKeyRunStatus], "running")
	}
	if patch.sets[stateKeyRunUpdatedAt] == "" {
		t.Error("stateKeyRunUpdatedAt is empty (B-06: updated_at not persisted)")
	}
	// Error message should not be set for non-error statuses
	if _, ok := patch.sets[stateKeyRunError]; ok {
		t.Errorf("stateKeyRunError should not be set for non-error status, got %q",
			patch.sets[stateKeyRunError])
	}
}

// TestPersistRunStatusToSession_TerminalStatus_PersistsStatusAndClearsRunID
// verifies that terminal statuses persist the status/error/updated_at and
// clear the run_id and await markers.
func TestPersistRunStatusToSession_TerminalStatus_PersistsStatusAndClearsRunID(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	// Pre-populate with a running state
	_ = sessions.PatchSessionState(context.Background(), "sess-1",
		map[string]string{
			stateKeyRunID:     "run-123",
			stateKeyRunStatus: "running",
		}, nil)

	ctx := context.Background()
	err := persistRunStatusToSession(sessions, ctx, "sess-1", "run-123", "completed", "")
	if err != nil {
		t.Fatalf("persistRunStatusToSession failed: %v", err)
	}

	patch := sessions.lastPatch()
	// Terminal status should still persist the status
	if patch.sets[stateKeyRunStatus] != "completed" {
		t.Errorf("stateKeyRunStatus=%q want %q (B-06: terminal status not persisted)",
			patch.sets[stateKeyRunStatus], "completed")
	}
	if patch.sets[stateKeyRunUpdatedAt] == "" {
		t.Error("stateKeyRunUpdatedAt is empty (B-06: updated_at not persisted for terminal status)")
	}
	// run_id should be cleared
	foundRunIDDelete := false
	for _, d := range patch.deletes {
		if d == stateKeyRunID {
			foundRunIDDelete = true
			break
		}
	}
	if !foundRunIDDelete {
		t.Error("stateKeyRunID should be in deletes for terminal status")
	}
}

// TestPersistRunStatusToSession_FailedStatus_PersistsErrorMessage verifies
// that when status is "failed", the error message is persisted.
func TestPersistRunStatusToSession_FailedStatus_PersistsErrorMessage(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	ctx := context.Background()
	errMsg := "LLM provider timeout"

	err := persistRunStatusToSession(sessions, ctx, "sess-1", "run-123", "failed", errMsg)
	if err != nil {
		t.Fatalf("persistRunStatusToSession failed: %v", err)
	}

	patch := sessions.lastPatch()
	if patch.sets[stateKeyRunStatus] != "failed" {
		t.Errorf("stateKeyRunStatus=%q want %q", patch.sets[stateKeyRunStatus], "failed")
	}
	if patch.sets[stateKeyRunError] != errMsg {
		t.Errorf("stateKeyRunError=%q want %q (B-06: error message not persisted)",
			patch.sets[stateKeyRunError], errMsg)
	}
}

// TestPersistRunStatusToSession_HydrateRoundTrip verifies that after
// persisting a run status, HydrateRunStatusFromSession can read it back.
// This is the core crash-recovery scenario.
func TestPersistRunStatusToSession_HydrateRoundTrip(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	ctx := context.Background()

	// Persist a "running" status
	if err := persistRunStatusToSession(sessions, ctx, "sess-1", "run-abc", "running", ""); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	// Read back via GetSessionState (simulates HydrateRunStatusFromSession)
	state, err := sessions.GetSessionState(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSessionState failed: %v", err)
	}

	status := strings.TrimSpace(state[stateKeyRunStatus])
	if status == "" {
		t.Fatal("B-06: stateKeyRunStatus not persisted; HydrateRunStatusFromSession would return false")
	}
	if status != "running" {
		t.Errorf("stateKeyRunStatus=%q want %q", status, "running")
	}
	if state[stateKeyRunID] != "run-abc" {
		t.Errorf("stateKeyRunID=%q want %q", state[stateKeyRunID], "run-abc")
	}
}

// TestPersistRunStatusToSession_EmptySessionID_NoOp verifies that empty
// session IDs are silently ignored (no panic, no DB call).
func TestPersistRunStatusToSession_EmptySessionID_NoOp(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	ctx := context.Background()

	err := persistRunStatusToSession(sessions, ctx, "", "run-123", "running", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sessions.patches) != 0 {
		t.Errorf("expected 0 patches, got %d", len(sessions.patches))
	}
}

// TestPersistRunStatusToSession_NilSessions_NoOp verifies that nil sessions
// are silently ignored.
func TestPersistRunStatusToSession_NilSessions_NoOp(t *testing.T) {
	ctx := context.Background()
	err := persistRunStatusToSession(nil, ctx, "sess-1", "run-123", "running", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPersistRunStatusToSession_UpdatedAtIsRFC3339 verifies that the
// updated_at timestamp is a valid RFC3339 timestamp.
func TestPersistRunStatusToSession_UpdatedAtIsRFC3339(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	ctx := context.Background()

	_ = persistRunStatusToSession(sessions, ctx, "sess-1", "run-123", "running", "")
	patch := sessions.lastPatch()

	updatedStr := patch.sets[stateKeyRunUpdatedAt]
	if updatedStr == "" {
		t.Fatal("stateKeyRunUpdatedAt is empty")
	}
	if _, err := time.Parse(time.RFC3339, updatedStr); err != nil {
		t.Errorf("stateKeyRunUpdatedAt=%q is not valid RFC3339: %v", updatedStr, err)
	}
}

// Ensure recordingSessionStatePort satisfies biz.SessionStatePort.
var _ biz.SessionStatePort = (*recordingSessionStatePort)(nil)
