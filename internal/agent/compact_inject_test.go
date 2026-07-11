package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/memory"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeManualCompressorAgent is a minimal biz.ManualCompressor for hook tests.
type fakeManualCompressorAgent struct{}

func (fakeManualCompressorAgent) CompactSession(context.Context, string, string) (*biz.CompactResult, error) {
	return &biz.CompactResult{Compacted: false}, nil
}

func TestCompactHook_NilCompressorReturnsNil(t *testing.T) {
	deps := TRPCBuilderDeps{}
	hook := newCompactContextBeforeHook(biz.Agent{}, deps)
	if hook != nil {
		t.Error("expected nil hook when ManualCompressor is nil")
	}
}

func TestCompactHook_InjectsForCompactTool(t *testing.T) {
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ManualCompressor: fakeManualCompressorAgent{},
		},
	}
	hook := newCompactContextBeforeHook(biz.Agent{}, deps)
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}

	beforeHook, ok := hook.(interface {
		HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
	})
	if !ok {
		t.Fatal("hook does not implement HandleBeforeTool")
	}

	// Build a context with an invocation that has a session.
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "test-session-id"},
	}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	args := &trpctool.BeforeToolArgs{
		Declaration: &trpctool.Declaration{Name: "compact"},
	}
	result, err := beforeHook.HandleBeforeTool(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Context == nil {
		t.Fatal("expected non-nil result with context")
	}
	if memory.ManualCompressorFromCtx(result.Context) == nil {
		t.Error("expected ManualCompressor to be injected")
	}
	if memory.CompactSessionIDFromCtx(result.Context) != "test-session-id" {
		t.Errorf("expected sessionID=test-session-id, got %q", memory.CompactSessionIDFromCtx(result.Context))
	}
}

func TestCompactHook_SkipsOtherTools(t *testing.T) {
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ManualCompressor: fakeManualCompressorAgent{},
		},
	}
	hook := newCompactContextBeforeHook(biz.Agent{}, deps)
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}

	beforeHook, ok := hook.(interface {
		HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
	})
	if !ok {
		t.Fatal("hook does not implement HandleBeforeTool")
	}

	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "test-session-id"},
	}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	args := &trpctool.BeforeToolArgs{
		Declaration: &trpctool.Declaration{Name: "memory_load"},
	}
	result, err := beforeHook.HandleBeforeTool(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if memory.ManualCompressorFromCtx(result.Context) != nil {
		t.Error("expected no ManualCompressor injection for non-compact tool")
	}
}

func TestCompactHook_SkipsWithoutSession(t *testing.T) {
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ManualCompressor: fakeManualCompressorAgent{},
		},
	}
	hook := newCompactContextBeforeHook(biz.Agent{}, deps)

	beforeHook := hook.(interface {
		HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
	})

	// No invocation in context
	args := &trpctool.BeforeToolArgs{
		Declaration: &trpctool.Declaration{Name: "compact"},
	}
	result, err := beforeHook.HandleBeforeTool(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if memory.ManualCompressorFromCtx(result.Context) != nil {
		t.Error("expected no injection without invocation session")
	}
}
