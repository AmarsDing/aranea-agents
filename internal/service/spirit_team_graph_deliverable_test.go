package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// ---------------------------------------------------------------------------
// Functional tests for the B.10.15.4 Graph StateFields bridge adapter:
// graphDeliverableReader against a REAL trpc in-memory session service.
// The biz-side bridge logic is covered by spirit_team_deliverable_test.go
// (stub reader); these tests verify the adapter seam itself — session-key
// coordinates (AppName = anchor agent ID), state decoding, and degradation
// contracts the biz fallback relies on.
// ---------------------------------------------------------------------------

func newGraphDeliverableRuntime(t *testing.T) (*araneasession.Runtime, trpcsession.Service) {
	t.Helper()
	svc := trpcinmemory.NewSessionService()
	return araneasession.NewRuntime(svc, loggateway.NewNoop()), svc
}

// F1: a session whose state carries the deliverable map decodes into a map
// with every key preserved (summary + structured extras).
func TestGraphDeliverableReader_ReadsDeliverableState(t *testing.T) {
	rt, svc := newGraphDeliverableRuntime(t)
	ctx := context.Background()
	key := trpcsession.Key{AppName: "agent-anchor", UserID: "default_user", SessionID: "sess-t1"}
	_, err := svc.CreateSession(ctx, key, trpcsession.StateMap{
		biz.DeliverableStateKey: []byte(`{"summary":"结构化摘要","confidence":0.9,"findings":["A","B"]}`),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	reader := NewGraphDeliverableReader(rt)
	got, err := reader.ReadGraphDeliverable(ctx, "agent-anchor", "default_user", "sess-t1")
	if err != nil {
		t.Fatalf("ReadGraphDeliverable: %v", err)
	}
	if got["summary"] != "结构化摘要" {
		t.Fatalf("summary mismatch: %v", got["summary"])
	}
	if _, ok := got["confidence"].(float64); !ok {
		t.Fatalf("confidence should decode as float64, got %T", got["confidence"])
	}
	if findings, ok := got["findings"].([]any); !ok || len(findings) != 2 {
		t.Fatalf("findings should decode as a 2-element array, got %v", got["findings"])
	}
}

// F2: the session exists but never wrote the deliverable key → (nil, nil),
// which the biz bridge treats as "state absent → reply fallback".
func TestGraphDeliverableReader_MissingDeliverableKey_ReturnsNilNil(t *testing.T) {
	rt, svc := newGraphDeliverableRuntime(t)
	ctx := context.Background()
	key := trpcsession.Key{AppName: "agent-anchor", UserID: "default_user", SessionID: "sess-t1"}
	if _, err := svc.CreateSession(ctx, key, trpcsession.StateMap{"other": []byte(`"v"`)}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := NewGraphDeliverableReader(rt).ReadGraphDeliverable(ctx, "agent-anchor", "default_user", "sess-t1")
	if err != nil || got != nil {
		t.Fatalf("missing deliverable key → (nil, nil), got (%v, %v)", got, err)
	}
}

// F3: a non-existent session must NOT surface an error — the in-memory
// service reports (nil, nil) and the adapter passes that through, so the biz
// fallback stays silent (no misleading warn) for anchor mismatches.
func TestGraphDeliverableReader_MissingSession_ReturnsNilNil(t *testing.T) {
	rt, _ := newGraphDeliverableRuntime(t)
	got, err := NewGraphDeliverableReader(rt).ReadGraphDeliverable(context.Background(), "agent-ghost", "default_user", "sess-gone")
	if err != nil || got != nil {
		t.Fatalf("missing session → (nil, nil), got (%v, %v)", got, err)
	}
}

// F4: corrupt deliverable JSON surfaces an error so the biz bridge logs a
// warn and falls back to reply extraction.
func TestGraphDeliverableReader_CorruptState_ReturnsError(t *testing.T) {
	rt, svc := newGraphDeliverableRuntime(t)
	ctx := context.Background()
	key := trpcsession.Key{AppName: "agent-anchor", UserID: "default_user", SessionID: "sess-t1"}
	if _, err := svc.CreateSession(ctx, key, trpcsession.StateMap{biz.DeliverableStateKey: []byte("{bad")}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if _, err := NewGraphDeliverableReader(rt).ReadGraphDeliverable(ctx, "agent-anchor", "default_user", "sess-t1"); err == nil {
		t.Fatalf("corrupt deliverable JSON must surface an error")
	}
}

// F5: nil runtime (v1-only deployment wiring) degrades to "state absent"
// without panicking.
func TestGraphDeliverableReader_NilRuntime_ReturnsNilNil(t *testing.T) {
	got, err := NewGraphDeliverableReader(nil).ReadGraphDeliverable(context.Background(), "agent-anchor", "default_user", "sess-t1")
	if err != nil || got != nil {
		t.Fatalf("nil runtime → (nil, nil), got (%v, %v)", got, err)
	}
}

// F6: blank session-key coordinates are rejected by CheckSessionKey — the
// biz layer's anchor probe must never pass empty values through.
func TestGraphDeliverableReader_BlankCoordinates_ReturnError(t *testing.T) {
	rt, _ := newGraphDeliverableRuntime(t)
	reader := NewGraphDeliverableReader(rt)
	for name, coords := range map[string][3]string{
		"blank appName":   {"", "default_user", "sess-t1"},
		"blank userID":    {"agent-anchor", "", "sess-t1"},
		"blank sessionID": {"agent-anchor", "default_user", ""},
	} {
		if _, err := reader.ReadGraphDeliverable(context.Background(), coords[0], coords[1], coords[2]); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

// F7: AppName isolation — state persisted under the anchor agent ID is
// invisible to any other AppName. This pins the bridge's core coordinate
// contract: the AppName MUST be the team's anchor agent ID, not the default
// app scope.
func TestGraphDeliverableReader_AppNameIsolation(t *testing.T) {
	rt, svc := newGraphDeliverableRuntime(t)
	ctx := context.Background()
	key := trpcsession.Key{AppName: "agent-anchor", UserID: "default_user", SessionID: "sess-t1"}
	_, err := svc.CreateSession(ctx, key, trpcsession.StateMap{
		biz.DeliverableStateKey: []byte(`{"summary":"锚点摘要"}`),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	reader := NewGraphDeliverableReader(rt)
	got, err := reader.ReadGraphDeliverable(ctx, "wrong-app", "default_user", "sess-t1")
	if err != nil || got != nil {
		t.Fatalf("other AppName must not see the anchor's state, got (%v, %v)", got, err)
	}
	got, err = reader.ReadGraphDeliverable(ctx, "agent-anchor", "default_user", "sess-t1")
	if err != nil || got["summary"] != "锚点摘要" {
		t.Fatalf("anchor AppName must read its own state, got (%v, %v)", got, err)
	}
}

// F8: user scope isolation — the same AppName/sessionID under a different
// user ID is a different trpc session entirely.
func TestGraphDeliverableReader_UserScopeIsolation(t *testing.T) {
	rt, svc := newGraphDeliverableRuntime(t)
	ctx := context.Background()
	key := trpcsession.Key{AppName: "agent-anchor", UserID: "user-a", SessionID: "sess-t1"}
	_, err := svc.CreateSession(ctx, key, trpcsession.StateMap{
		biz.DeliverableStateKey: []byte(`{"summary":"A 的摘要"}`),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	reader := NewGraphDeliverableReader(rt)
	got, err := reader.ReadGraphDeliverable(ctx, "agent-anchor", "user-b", "sess-t1")
	if err != nil || got != nil {
		t.Fatalf("other user must not see the state, got (%v, %v)", got, err)
	}
}
