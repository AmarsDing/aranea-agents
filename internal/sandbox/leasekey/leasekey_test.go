package leasekey

import (
	"context"
	"testing"

	"aranea-agents/internal/sandbox"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func ctxWithInvocation(agentName string) context.Context {
	inv := &trpcagent.Invocation{
		AgentName: agentName,
		Session:   &trpcsession.Session{ID: "sess-1", AppName: "app", UserID: "user"},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func TestFromContext_NonTeamMatchesLegacyRule(t *testing.T) {
	// 无 RunID：键与旧规则一致（app/user/sessionID，无成员后缀）。
	ctx := ctxWithInvocation("worker-a")
	if got := FromContext(ctx, "exec-1"); got != "app/user/sess-1" {
		t.Fatalf("non-team ctx: got %q", got)
	}
}

func TestFromContext_TeamRunMemberDimension(t *testing.T) {
	base := ctxWithInvocation("worker-a")
	ctx := sandbox.WithRunID(base, "run-1")

	got := FromContext(ctx, "")
	want := "app/user/sess-1#run:run-1#agent:worker-a"
	if got != want {
		t.Fatalf("team ctx: got %q, want %q", got, want)
	}

	// 同 run 同成员：键稳定（同成员多轮共享沙箱）。
	if again := FromContext(ctx, ""); again != got {
		t.Fatalf("same member must be stable: %q vs %q", again, got)
	}

	// 同 run 不同成员：键不同（成员级并行，各持沙箱）。
	other := sandbox.WithRunID(ctxWithInvocation("worker-b"), "run-1")
	if otherKey := FromContext(other, ""); otherKey == got {
		t.Fatalf("different members must not share key: %q", otherKey)
	}

	// 不同 run 同成员：键不同（跨 run 不复用沙箱）。
	otherRun := sandbox.WithRunID(ctxWithInvocation("worker-a"), "run-2")
	if otherRunKey := FromContext(otherRun, ""); otherRunKey == got {
		t.Fatalf("different runs must not share key: %q", otherRunKey)
	}
}

func TestFromContext_RunIDWithoutAgentNameKeepsLegacyKey(t *testing.T) {
	// RunID 存在但 AgentName 空（非成员调用，如团队级锚点）：不追加成员维度。
	ctx := sandbox.WithRunID(ctxWithInvocation(""), "run-1")
	if got := FromContext(ctx, ""); got != "app/user/sess-1" {
		t.Fatalf("run without agent name: got %q", got)
	}
}

func TestFromContext_NoInvocationFallsBackToExecutionID(t *testing.T) {
	if got := FromContext(context.Background(), "  exec-9  "); got != "exec-9" {
		t.Fatalf("no invocation: got %q", got)
	}
	if got := FromContext(context.Background(), "  "); got != "" {
		t.Fatalf("no invocation + blank executionID: got %q", got)
	}
	// invocation 存在但 session 空：同样回退 executionID。
	invCtx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{})
	if got := FromContext(invCtx, "exec-7"); got != "exec-7" {
		t.Fatalf("no session: got %q", got)
	}
}
