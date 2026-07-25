package session

import (
	"context"
	"strings"
	"testing"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
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

// findUserContextEvent returns the content of the first user-authored response
// event whose message content contains want, or "" when none matches.
func findUserContextEvent(events []trpcevent.Event, want string) string {
	for i := range events {
		e := &events[i]
		if e.Author != "user" || e.Response == nil {
			continue
		}
		for _, ch := range e.Response.Choices {
			if strings.Contains(ch.Message.Content, want) {
				return ch.Message.Content
			}
		}
	}
	return ""
}

// AppendUserContextEvent must inject the decision as a user-authored event so
// subsequent turns include it in the LLM request history — the same mechanism
// the runner uses for real user messages (runner.appendIncomingMessage).
func TestRuntime_AppendUserContextEvent_appendsToExistingSession(t *testing.T) {
	svc := trpcinmemory.NewSessionService()
	rt := NewRuntime(svc, loggateway.NewNoop())
	ctx := context.Background()
	userID := "default_user"
	sessionID := "sess-ctx-1"
	if _, err := svc.CreateSession(ctx, Key(userID, sessionID), nil); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	const text = "【计划确认】用户已批准执行计划（策略：dag，共 2 个子任务）。"
	if err := rt.AppendUserContextEvent(ctx, userID, sessionID, "plan-confirm:p1", text); err != nil {
		t.Fatalf("AppendUserContextEvent: %v", err)
	}

	sess, err := rt.Get(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got := findUserContextEvent(sess.Events, "计划确认")
	if got != text {
		t.Fatalf("user context event content = %q, want %q", got, text)
	}
}

// When the in-memory session is gone (process restart / first decision before
// any turn), the primitive must create it so the injected decision survives
// until the next turn instead of being silently dropped.
func TestRuntime_AppendUserContextEvent_createsMissingSession(t *testing.T) {
	rt := NewRuntime(trpcinmemory.NewSessionService(), loggateway.NewNoop())
	ctx := context.Background()

	const text = "【计划确认】用户拒绝了执行计划。"
	if err := rt.AppendUserContextEvent(ctx, "default_user", "sess-ctx-new", "plan-confirm:p2", text); err != nil {
		t.Fatalf("AppendUserContextEvent: %v", err)
	}

	sess, err := rt.Get(ctx, "default_user", "sess-ctx-new")
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if got := findUserContextEvent(sess.Events, "计划确认"); got != text {
		t.Fatalf("user context event content = %q, want %q", got, text)
	}
}

// Empty/whitespace content carries no semantics; injecting it would pollute
// the LLM history with an empty user turn. Must be rejected before any write.
func TestRuntime_AppendUserContextEvent_rejectsEmptyContent(t *testing.T) {
	svc := trpcinmemory.NewSessionService()
	rt := NewRuntime(svc, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := svc.CreateSession(ctx, Key("default_user", "sess-ctx-empty"), nil); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := rt.AppendUserContextEvent(ctx, "default_user", "sess-ctx-empty", "plan-confirm:p3", "   "); err == nil {
		t.Fatal("expected error for empty content")
	}

	sess, err := rt.Get(ctx, "default_user", "sess-ctx-empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(sess.Events) != 0 {
		t.Fatalf("no event must be appended for empty content, got %d", len(sess.Events))
	}
}
