package agent

import (
	"aranea-agents/internal/biz"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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

func TestConfirmTimeoutForTool(t *testing.T) {
	if confirmTimeoutForTool("subagents_spawn", 0) != 2*time.Minute {
		t.Fatal("spawn ttl")
	}
	if confirmTimeoutForTool("shell_exec", 0) != defaultToolConfirmationTimeout {
		t.Fatal("shell default ttl")
	}
	if confirmTimeoutForTool("subagents_spawn", 50*time.Millisecond) != 50*time.Millisecond {
		t.Fatal("override wins")
	}
}

func TestExtraConfirmAttemptsForSpawn(t *testing.T) {
	if extraConfirmAttemptsForTool("subagents_spawn", 0) != 0 {
		t.Fatal("spawn must not get the default extra confirm wait")
	}
	if extraConfirmAttemptsForTool("shell_exec", 0) != 1 {
		t.Fatal("non-spawn keeps default extra attempt")
	}
	if !strings.Contains(toolConfirmTimeoutMessage("subagents_spawn", 1, 2*time.Minute), "不要再次调用") {
		t.Fatal("spawn timeout copy must forbid retry")
	}
	if timeoutBlockCause("subagents_spawn") != "timeout_degrade" {
		t.Fatal("spawn timeout cause")
	}
}

// TestSpawnTimeoutDoesNotGrantSession pins F3: 同参 spawn「第 3 次 5ms 放行」
// 只允许来自首次 *approve* 后的 session batch grant（有 grant_session 留痕）。
// 超时不得 GrantSession，后续 spawn 仍须走确认卡。
func TestSpawnTimeoutDoesNotGrantSession(t *testing.T) {
	sessionGrants := newToolGrantStore(time.Now)
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"subagents_spawn": {requiresConfirm: true}},
		sessionGrants: sessionGrants,
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	h.confirmTimeout = 20 * time.Millisecond
	ctx := grantTestCtx("sess-1", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	res, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "subagents_spawn", ToolCallID: "c1"})
	if err != nil {
		t.Fatalf("timeout must reject without callback error: %v", err)
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "不要再次调用") {
		t.Fatalf("spawn timeout copy: %q", msg)
	}
	if sessionGrants.HasSession("sess-1", "agent-1", "subagents_spawn") {
		t.Fatal("timeout must not session-grant remaining spawns")
	}
	waited := false
	ctx2 := grantTestCtx("sess-1", func(ctx context.Context) (string, error) {
		waited = true
		<-ctx.Done()
		return "", ctx.Err()
	})
	if _, err := h.HandleBeforeTool(ctx2, &trpctool.BeforeToolArgs{ToolName: "subagents_spawn", ToolCallID: "c2"}); err != nil {
		t.Fatal(err)
	}
	if !waited {
		t.Fatal("second spawn after timeout must still wait for confirm")
	}
}

func TestHITLWaitVisibilityEmitsNotice(t *testing.T) {
	h := newToolConfirmationBeforeHook(nil, biz.Agent{ID: "a"}, TRPCBuilderDeps{})
	h.hitlVisibleAfter = 20 * time.Millisecond
	emitter := &fakeFactoryEmitter{}
	stop := h.startHITLWaitVisibility(context.Background(), "subagents_spawn", emitter)
	time.Sleep(80 * time.Millisecond)
	stop()
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	found := false
	for _, n := range emitter.notices {
		if n == hitlWaitNoticeType {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected hitl_wait notice, got %v", emitter.notices)
	}
}

func TestToolConfirmationHook_SpawnCoalescesParallelCards(t *testing.T) {
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"subagents_spawn": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1"}, TRPCBuilderDeps{})
	emitter := &fakeFactoryEmitter{}
	started := make(chan struct{})
	release := make(chan struct{})
	ctx := grantTestCtx("sess-1", func(ctx context.Context) (string, error) {
		close(started)
		<-release
		return "approved", nil
	})
	ctx = biz.WithActivityEmitter(ctx, emitter)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "subagents_spawn", ToolCallID: "c1"})
		errCh <- err
	}()
	<-started
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "subagents_spawn", ToolCallID: "c2"})
		errCh <- err
	}()
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("coalesced spawn confirm err: %v", err)
		}
	}
	if n := len(emitter.confirmParams); n != 1 {
		t.Fatalf("confirm cards=%d want 1", n)
	}
	if !gate.sessionGrants.HasSession("sess-1", "agent-1", "subagents_spawn") {
		t.Fatal("first spawn approve must session-grant remaining spawns")
	}
}
