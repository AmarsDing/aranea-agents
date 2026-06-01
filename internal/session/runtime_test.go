package session

import (
	"context"
	"testing"

	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"

	"aranea-agents/pkg/loggateway"
)

func TestRuntime_SyncRunnerSnapshot_updatesState(t *testing.T) {
	svc := trpcinmemory.NewSessionService()
	rt := NewRuntime(svc, loggateway.NewNoop())
	ctx := context.Background()
	userID := "default_user"
	sessionID := "sess-sync-1"

	k := Key(userID, sessionID)
	if _, err := svc.CreateSession(ctx, k, nil); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	snapshot := `{"events":[{"role":"user","content":"hi"}]}`
	if err := rt.SyncRunnerSnapshot(ctx, userID, sessionID, snapshot, "summary-md"); err != nil {
		t.Fatalf("SyncRunnerSnapshot: %v", err)
	}

	sess, err := rt.Get(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	raw, ok := sess.State[stateKeyRunnerSnapshot]
	if !ok || string(raw) != snapshot {
		t.Fatalf("runner snapshot state = %q ok=%v", string(raw), ok)
	}
	md, ok := sess.State[stateKeyCompressedSummary]
	if !ok || string(md) != "summary-md" {
		t.Fatalf("summary state = %q ok=%v", string(md), ok)
	}
}

func TestRuntime_SyncStateDelta_updatesKV(t *testing.T) {
	svc := trpcinmemory.NewSessionService()
	rt := NewRuntime(svc, loggateway.NewNoop())
	ctx := context.Background()
	userID := "default_user"
	sessionID := "sess-kv-1"
	k := Key(userID, sessionID)
	if _, err := svc.CreateSession(ctx, k, nil); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := rt.SyncStateDelta(ctx, userID, sessionID, "set", "todo", `{"done":true}`); err != nil {
		t.Fatalf("SyncStateDelta: %v", err)
	}
	sess, err := rt.Get(ctx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := sess.State[stateKVKey("todo")]
	if !ok || string(raw) != `{"done":true}` {
		t.Fatalf("state = %q ok=%v", string(raw), ok)
	}
}

func TestRuntime_SyncRunnerSnapshot_missingSession(t *testing.T) {
	rt := NewRuntime(trpcinmemory.NewSessionService(), loggateway.NewNoop())
	err := rt.SyncRunnerSnapshot(context.Background(), "default_user", "missing", `{}`, "")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}
