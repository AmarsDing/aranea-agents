package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeToolGrantStore is an in-memory biz.ToolGrantStore for hook tests.
type fakeToolGrantStore struct {
	grants map[string]biztool.ToolGrant
}

func newFakeToolGrantStore() *fakeToolGrantStore {
	return &fakeToolGrantStore{grants: make(map[string]biztool.ToolGrant)}
}

func fakeGrantKey(agentID, toolKey string) string { return agentID + "|" + toolKey }

func (f *fakeToolGrantStore) HasToolGrant(_ context.Context, agentID, toolKey string) (bool, error) {
	_, ok := f.grants[fakeGrantKey(agentID, toolKey)]
	return ok, nil
}

func (f *fakeToolGrantStore) ListToolGrants(_ context.Context, agentID string) ([]biztool.ToolGrant, error) {
	var out []biztool.ToolGrant
	for _, g := range f.grants {
		if g.AgentID == agentID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeToolGrantStore) CreateToolGrant(_ context.Context, g biztool.ToolGrant) error {
	f.grants[fakeGrantKey(g.AgentID, g.ToolKey)] = g
	return nil
}

func (f *fakeToolGrantStore) DeleteToolGrant(_ context.Context, agentID, toolKey string) error {
	delete(f.grants, fakeGrantKey(agentID, toolKey))
	return nil
}

// newGrantTestHook builds a confirmation hook whose gate requires
// confirmation for "bash" and is backed by the given persisted grant store.
func newGrantTestHook(t *testing.T, store *fakeToolGrantStore) (*toolConfirmationBeforeHook, *toolGrantStore) {
	t.Helper()
	uc := biztool.NewToolUsecase(nil, nil, loggateway.NewNoop(), biztool.WithToolGrantStore(store))
	sessionGrants := newToolGrantStore(time.Now)
	deps := TRPCBuilderDeps{}
	deps.ToolUC = uc
	gate := &toolConfirmGate{
		catalog:        map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants:  sessionGrants,
		persistedGrant: uc.HasToolGrant,
	}
	return newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, deps), sessionGrants
}

func grantTestCtx(sessionID string, reply serviceawaitreply.ReplyFunc) context.Context {
	var ctx context.Context = trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: sessionID},
	})
	if reply != nil {
		ctx = serviceawaitreply.WithReplyFunc(ctx, reply)
	}
	return ctx
}

func bashToolArgs(callID string) *trpctool.BeforeToolArgs {
	return &trpctool.BeforeToolArgs{ToolName: "bash", ToolCallID: callID}
}

func TestToolConfirmationHook_SessionGrantSkipsPrompt(t *testing.T) {
	h, sessionGrants := newGrantTestHook(t, newFakeToolGrantStore())
	sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		t.Fatal("ReplyFunc must not be called when a session grant exists")
		return "", errors.New("unexpected call")
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil {
		t.Fatalf("HandleBeforeTool err = %v", err)
	}
	if res == nil || res.Context == nil {
		t.Fatal("expected allow result, got nil")
	}
}

func TestToolConfirmationHook_PersistedGrantSkipsPrompt(t *testing.T) {
	store := newFakeToolGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatal(err)
	}
	h, _ := newGrantTestHook(t, store)
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		t.Fatal("ReplyFunc must not be called when a persisted grant exists")
		return "", errors.New("unexpected call")
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil || res == nil {
		t.Fatalf("HandleBeforeTool = (%v,%v), want allow", res, err)
	}
}

func TestToolConfirmationHook_ApproveSessionGrantsAndSecondCallSkips(t *testing.T) {
	h, sessionGrants := newGrantTestHook(t, newFakeToolGrantStore())
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApproveSession, nil
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil || res == nil {
		t.Fatalf("first call = (%v,%v), want allow", res, err)
	}
	if !sessionGrants.HasSession("sess-1", "agent-1", "bash") {
		t.Fatal("expected session grant recorded after approve_session reply")
	}
	ctx2 := grantTestCtx("sess-1", func(context.Context) (string, error) {
		t.Fatal("ReplyFunc must not be called after session grant")
		return "", errors.New("unexpected call")
	})
	res2, err2 := h.HandleBeforeTool(ctx2, bashToolArgs("call-2"))
	if err2 != nil || res2 == nil {
		t.Fatalf("second call = (%v,%v), want allow", res2, err2)
	}
}

func TestToolConfirmationHook_ApproveAlwaysPersistsGrant(t *testing.T) {
	store := newFakeToolGrantStore()
	h, _ := newGrantTestHook(t, store)
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApproveAlways, nil
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil || res == nil {
		t.Fatalf("call = (%v,%v), want allow", res, err)
	}
	has, herr := store.HasToolGrant(context.Background(), "agent-1", "bash")
	if herr != nil || !has {
		t.Fatalf("persisted grant = (%v,%v), want (true,nil)", has, herr)
	}
}

func TestToolConfirmationHook_DenyStillBlocks(t *testing.T) {
	// Deny path records the blocked invocation via recordToolInvocationWrite;
	// keep ToolUC nil so the recorder short-circuits without a DB repo.
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyDeny, nil
	})
	_, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err == nil || !strings.Contains(err.Error(), errToolConfirmationRequired) {
		t.Fatalf("deny err = %v, want %s", err, errToolConfirmationRequired)
	}
}

func TestToolConfirmationHook_GrantDoesNotLeakAcrossAgents(t *testing.T) {
	store := newFakeToolGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatal(err)
	}
	// Another agent must still be prompted: persisted grants are keyed by
	// (agentID, toolKey), so agent-2 never matches agent-1's grant.
	gate2 := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
		persistedGrant: func(_ context.Context, agentID, toolKey string) bool {
			has, _ := store.HasToolGrant(context.Background(), agentID, toolKey)
			return has
		},
	}
	h2 := newToolConfirmationBeforeHook(gate2, biz.Agent{ID: "agent-2"}, TRPCBuilderDeps{})
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyDeny, nil
	})
	_, err := h2.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err == nil {
		t.Fatal("agent-2 must not inherit agent-1's persisted grant")
	}
}
