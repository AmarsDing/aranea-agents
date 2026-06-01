package biz

import (
	"context"
	"errors"
	"testing"
)

type stubRunnerSnapshotSync struct {
	calls int
	last  struct {
		userID, sessionID, snapshot, summary string
		op, path, value                      string
	}
	err error
}

func (s *stubRunnerSnapshotSync) SyncRunnerSnapshot(_ context.Context, userID, sessionID, snapshotJSON, summaryMarkdown string) error {
	s.calls++
	s.last.userID = userID
	s.last.sessionID = sessionID
	s.last.snapshot = snapshotJSON
	s.last.summary = summaryMarkdown
	return s.err
}

func (s *stubRunnerSnapshotSync) SyncStateDelta(_ context.Context, userID, sessionID, operation, path, valueJSON string) error {
	s.calls++
	s.last.userID = userID
	s.last.sessionID = sessionID
	s.last.op = operation
	s.last.path = path
	s.last.value = valueJSON
	return s.err
}

type recordingSessionLogger struct {
	errors []string
}

func (l *recordingSessionLogger) LogSessionError(_ context.Context, _ string, stepID, message string, _ ...LogPair) {
	l.errors = append(l.errors, stepID+":"+message)
}
func (l *recordingSessionLogger) LogSessionWarn(context.Context, string, string, string, ...LogPair)  {}
func (l *recordingSessionLogger) LogSessionInfo(context.Context, string, string, string, ...LogPair)  {}
func (l *recordingSessionLogger) LogSessionDebug(context.Context, string, string, string, ...LogPair) {}

func TestStateDeltaHandler_syncRunnerSnapshot(t *testing.T) {
	sync := &stubRunnerSnapshotSync{}
	h := &stateDeltaHandler{runnerSync: sync}

	h.syncRunnerSnapshot(context.Background(), "sess-1", `{"events":[]}`, "summary")

	if sync.calls != 1 {
		t.Fatalf("sync calls = %d want 1", sync.calls)
	}
	if sync.last.userID != "default_user" {
		t.Fatalf("user_id = %q want default_user", sync.last.userID)
	}
	if sync.last.sessionID != "sess-1" {
		t.Fatalf("session_id = %q", sync.last.sessionID)
	}
	if sync.last.snapshot != `{"events":[]}` {
		t.Fatalf("snapshot = %q", sync.last.snapshot)
	}
}

func TestStateDeltaHandler_syncStateDelta_logsError(t *testing.T) {
	sync := &stubRunnerSnapshotSync{err: errors.New("boom")}
	logger := &recordingSessionLogger{}
	h := &stateDeltaHandler{runnerSync: sync, logger: logger}

	h.syncStateDelta(context.Background(), "sess-2", "set", "todo", `{"done":true}`)

	if len(logger.errors) == 0 {
		t.Fatal("expected error log")
	}
}

func TestStateDeltaHandler_syncRunnerSnapshot_nilSync(t *testing.T) {
	h := &stateDeltaHandler{}
	h.syncRunnerSnapshot(context.Background(), "sess-1", `{}`, "") // must not panic
}
