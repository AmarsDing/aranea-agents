package biz

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testAutoMemoryEnqueuer captures enqueued jobs for test assertions.
type testAutoMemoryEnqueuer struct {
	mu   sync.Mutex
	jobs []struct {
		AppName    string
		SessionID  string
		EnqueuedAt time.Time
	}
}

func (e *testAutoMemoryEnqueuer) EnqueueAutoMemory(appName, sessionID string, enqueuedAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobs = append(e.jobs, struct {
		AppName    string
		SessionID  string
		EnqueuedAt time.Time
	}{appName, sessionID, enqueuedAt})
}

func (e *testAutoMemoryEnqueuer) lastJob() (appName, sessionID string, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.jobs) == 0 {
		return "", "", false
	}
	j := e.jobs[len(e.jobs)-1]
	return j.AppName, j.SessionID, true
}

type noopSessionLogWriter struct{}

func (noopSessionLogWriter) LogSessionWarn(_ context.Context, _, _, _ string, _ ...LogPair)  {}
func (noopSessionLogWriter) LogSessionError(_ context.Context, _, _, _ string, _ ...LogPair) {}

func TestTurnMemoryWorker_OnRunnerCompletion_EnqueuesJob(t *testing.T) {
	enqueuer := &testAutoMemoryEnqueuer{}
	w := NewTurnMemoryWorker(enqueuer, nil, noopSessionLogWriter{})
	w.OnRunnerCompletion(context.Background(), DomainEvent{SessionID: "sess-1", Author: "agent-a"})

	appName, sessionID, ok := enqueuer.lastJob()
	if !ok {
		t.Fatal("expected auto-memory job")
	}
	if sessionID != "sess-1" || appName != "agent-a" {
		t.Fatalf("unexpected job: appName=%s sessionID=%s", appName, sessionID)
	}
}

func TestTurnMemoryWorker_OnRunnerCompletion_SkipsEmptySession(t *testing.T) {
	enqueuer := &testAutoMemoryEnqueuer{}
	w := NewTurnMemoryWorker(enqueuer, nil, noopSessionLogWriter{})
	w.OnRunnerCompletion(context.Background(), DomainEvent{SessionID: "  ", Author: "agent-a"})

	_, _, ok := enqueuer.lastJob()
	if ok {
		t.Fatal("unexpected job enqueued for empty session")
	}
}
