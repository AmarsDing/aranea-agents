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

// TestToolConfirmGate_ConfirmationMapSkipsPersistedGrants verifies the
// declaration-annotation tier: tools with a persisted (agentID, toolKey)
// grant must NOT be annotated "requires confirmation", so unattended graph
// runs don't see the annotation and stall waiting for approval. Tools
// without a grant keep the annotation.
func TestToolConfirmGate_ConfirmationMapSkipsPersistedGrants(t *testing.T) {
	store := newFakeToolGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatal(err)
	}
	gate := &toolConfirmGate{
		catalog: map[string]confirmCatalogEntry{
			"bash":       {requiresConfirm: true},
			"shell_exec": {requiresConfirm: true},
		},
		sessionGrants: newToolGrantStore(time.Now),
		agentID:       "agent-1",
		persistedGrant: func(_ context.Context, agentID, toolKey string) bool {
			has, _ := store.HasToolGrant(context.Background(), agentID, toolKey)
			return has
		},
	}
	m := gate.confirmationMap(context.Background())
	if m["bash"] {
		t.Fatal("bash has a persisted grant and must not be annotated")
	}
	if !m["shell_exec"] {
		t.Fatal("shell_exec has no grant and must stay annotated")
	}

	// A different agent (no grants) sees both annotations.
	gate2 := *gate
	gate2.agentID = "agent-2"
	m2 := gate2.confirmationMap(context.Background())
	if !m2["bash"] || !m2["shell_exec"] {
		t.Fatalf("agent-2 must see both annotations, got %v", m2)
	}
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
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	// P1-3: 用户拒绝是显式 Reject 决策（CustomResult 短路），不是回调错误——
	// error 语义保留给拦截器自身故障。
	if err != nil {
		t.Fatalf("deny must not surface as a callback error, got %v", err)
	}
	msg, ok := res.CustomResult.(string)
	if !ok || !strings.Contains(msg, errToolConfirmationRequired) {
		t.Fatalf("deny CustomResult = %v, want string containing %s", res.CustomResult, errToolConfirmationRequired)
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
	res2, err2 := h2.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err2 != nil {
		t.Fatalf("deny must not surface as a callback error, got %v", err2)
	}
	if res2 == nil || res2.CustomResult == nil {
		t.Fatal("agent-2 must not inherit agent-1's persisted grant (expect explicit Reject)")
	}
}

// TestToolConfirmationHook_DenyMessageIsSemantic verifies the LLM-facing deny
// message is a semantic user-decision instruction, not a raw internal error
// code. The model must be able to tell "user deliberately rejected" apart
// from a retriable system failure, and must be told not to retry.
// The errToolConfirmationRequired prefix must be preserved because
// tool_invocation_recorder identifies confirmation errors by it.
// P1-3: the message is delivered via CustomResult (explicit Reject).
func TestToolConfirmationHook_DenyMessageIsSemantic(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyDeny, nil
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil {
		t.Fatalf("deny must not surface as a callback error, got %v", err)
	}
	msg, ok := res.CustomResult.(string)
	if !ok {
		t.Fatalf("deny CustomResult = %v, want string", res.CustomResult)
	}
	if !strings.Contains(msg, errToolConfirmationRequired) {
		t.Fatalf("prefix lost: %q", msg)
	}
	if !strings.Contains(msg, "用户拒绝") {
		t.Fatalf("deny message not semantic: %q", msg)
	}
	if !strings.Contains(msg, "bash") {
		t.Fatalf("deny message lacks tool key: %q", msg)
	}
	if !strings.Contains(msg, "不要重试") && !strings.Contains(msg, "禁止重试") {
		t.Fatalf("deny message lacks no-retry guidance: %q", msg)
	}
}

// TestToolConfirmationHook_UnavailableMessageIsSemantic verifies the
// no-reply-channel path tells the LLM that confirmation is required but
// cannot be requested in the current environment.
func TestToolConfirmationHook_UnavailableMessageIsSemantic(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	ctx := grantTestCtx("sess-1", nil) // no reply func → unavailable path
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil {
		t.Fatalf("unavailable path must not surface as a callback error, got %v", err)
	}
	msg, ok := res.CustomResult.(string)
	if !ok {
		t.Fatalf("unavailable CustomResult = %v, want string", res.CustomResult)
	}
	if !strings.Contains(msg, errToolConfirmationRequired) {
		t.Fatalf("prefix lost: %q", msg)
	}
	if !strings.Contains(msg, "需要用户确认") {
		t.Fatalf("unavailable message not semantic: %q", msg)
	}
}

// TestToolConfirmationHook_TimeoutMessageIsSemantic verifies the LLM-facing
// timeout message explains that the user did not respond (not a rejection)
// and tells the model to ask the user before retrying.
func TestToolConfirmationHook_TimeoutMessageIsSemantic(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	h.confirmTimeout = 20 * time.Millisecond
	ctx := grantTestCtx("sess-1", func(ctx context.Context) (string, error) {
		// Block until the confirmation deadline expires.
		<-ctx.Done()
		return "", ctx.Err()
	})
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil {
		t.Fatalf("timeout path must not surface as a callback error, got %v", err)
	}
	msg, ok := res.CustomResult.(string)
	if !ok {
		t.Fatalf("timeout CustomResult = %v, want string", res.CustomResult)
	}
	if !strings.Contains(msg, errToolConfirmationRequired) {
		t.Fatalf("prefix lost: %q", msg)
	}
	if !strings.Contains(msg, "超时") || !strings.Contains(msg, "不代表用户拒绝") {
		t.Fatalf("timeout message not semantic: %q", msg)
	}
	if !strings.Contains(msg, "可随时批准") && !strings.Contains(msg, "可以重试") {
		t.Fatalf("timeout after retry must stay retryable, not silent cancel: %q", msg)
	}
}

func TestToolConfirmationHook_TimeoutReissuesConfirmOnce(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	h.confirmTimeout = 20 * time.Millisecond
	emitter := &fakeFactoryEmitter{}
	calls := 0
	ctx := grantTestCtx("sess-1", func(ctx context.Context) (string, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return "approved", nil
	})
	ctx = biz.WithActivityEmitter(ctx, emitter)
	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-retry"))
	if err != nil {
		t.Fatalf("retry-then-approve must not error: %v", err)
	}
	if res == nil {
		t.Fatal("expected allow result after retry approval")
	}
	if calls != 2 {
		t.Fatalf("reply waits=%d want 2 (timeout then retry)", calls)
	}
	if len(emitter.confirmParams) != 2 {
		t.Fatalf("confirm cards=%d want 2", len(emitter.confirmParams))
	}
	if len(emitter.confirmTimeouts) != 1 {
		t.Fatalf("timeout emits=%d want 1", len(emitter.confirmTimeouts))
	}
	if len(emitter.confirmTimeoutRetrying) != 1 || !emitter.confirmTimeoutRetrying[0] {
		t.Fatal("first timeout must be tagged retrying so the UI does not look like a silent cancel")
	}
}

// TestToolConfirmationHook_ConfirmRequestCarriesAgentKey verifies the confirm
// Activity is attributed to the agent whose tool is gated (h.ag.AgentKey), so
// that in team graph mode the member's confirm step attaches to the member's
// activity panel instead of the anchor agent.
func TestToolConfirmationHook_ConfirmRequestCarriesAgentKey(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit-worker-a"}, TRPCBuilderDeps{})
	emitter := &fakeFactoryEmitter{}
	ctx := grantTestCtx("sess-1", func(context.Context) (string, error) {
		return "approved", nil
	})
	ctx = biz.WithActivityEmitter(ctx, emitter)

	res, err := h.HandleBeforeTool(ctx, bashToolArgs("call-1"))
	if err != nil {
		t.Fatalf("HandleBeforeTool err = %v", err)
	}
	if res == nil {
		t.Fatal("expected allow result after approval, got nil")
	}
	if len(emitter.confirmParams) != 1 {
		t.Fatalf("confirmRequests=%d want 1", len(emitter.confirmParams))
	}
	if got := emitter.confirmParams[0].AuthorAgentKey; got != "spirit-worker-a" {
		t.Fatalf("AuthorAgentKey=%q want %q", got, "spirit-worker-a")
	}
}
