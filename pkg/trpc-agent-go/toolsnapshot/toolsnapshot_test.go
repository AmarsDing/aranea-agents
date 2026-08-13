package toolsnapshot

import (
	"context"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	internalsnapshot "trpc.group/trpc-go/trpc-agent-go/internal/flow/toolsnapshot"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubTool struct{ name string }

func (s *stubTool) Declaration() *tool.Declaration {
	return &tool.Declaration{Name: s.name}
}

func TestInvalidateClearsSnapshot(t *testing.T) {
	inv := agent.NewInvocation()
	internalsnapshot.Set(inv, []tool.Tool{&stubTool{name: "a"}}, true)
	if _, ok := internalsnapshot.Get(inv); !ok {
		t.Fatal("expected snapshot to be cached before invalidate")
	}
	Invalidate(inv)
	if _, ok := internalsnapshot.Get(inv); ok {
		t.Fatal("expected snapshot to be cleared after Invalidate")
	}
	if _, ok := internalsnapshot.HasFilteredUserTools(inv); ok {
		t.Fatal("expected has-filtered-user-tools flag to be cleared")
	}
}

func TestInvalidateNilInvocationIsNoop(t *testing.T) {
	Invalidate(nil) // must not panic
}

func TestInvalidateFromContext(t *testing.T) {
	if InvalidateFromContext(context.Background()) {
		t.Fatal("expected false when ctx carries no invocation")
	}

	inv := agent.NewInvocation()
	internalsnapshot.Set(inv, []tool.Tool{&stubTool{name: "b"}}, false)
	ctx := agent.NewInvocationContext(context.Background(), inv)
	if !InvalidateFromContext(ctx) {
		t.Fatal("expected true when ctx carries an invocation")
	}
	if _, ok := internalsnapshot.Get(inv); ok {
		t.Fatal("expected snapshot to be cleared via InvalidateFromContext")
	}
}
