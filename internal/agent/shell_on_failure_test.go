package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestExtractToolExitCode(t *testing.T) {
	t.Parallel()
	if _, ok := extractToolExitCode("not a map"); ok {
		t.Fatal("non-map must miss")
	}
	if _, ok := extractToolExitCode(map[string]any{"status": "running"}); ok {
		t.Fatal("missing exit_code must miss")
	}
	code, ok := extractToolExitCode(map[string]any{"exit_code": 2})
	if !ok || code != 2 {
		t.Fatalf("int exit_code = (%d,%v)", code, ok)
	}
	code, ok = extractToolExitCode(map[string]any{"exit_code": float64(1)})
	if !ok || code != 1 {
		t.Fatalf("float64 exit_code = (%d,%v)", code, ok)
	}
	code, ok = extractToolExitCode(map[string]any{
		"truncated": true,
		"content":   `{"exit_code":3,"output":"boom"}`,
	})
	if !ok || code != 3 {
		t.Fatalf("truncated content exit_code = (%d,%v)", code, ok)
	}
}

func TestRecordShellOnFailure_ArmsAndClears(t *testing.T) {
	t.Parallel()
	inv := &trpcagent.Invocation{}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	recordShellOnFailure(ctx, &trpctool.AfterToolArgs{
		ToolName: "save_file",
		Result:   map[string]any{"exit_code": 1},
	})
	if shellOnFailureArmed(ctx) {
		t.Fatal("non-shell must not arm")
	}

	recordShellOnFailure(ctx, &trpctool.AfterToolArgs{
		ToolName: "shell_exec",
		Result:   map[string]any{"status": "running"},
	})
	if shellOnFailureArmed(ctx) {
		t.Fatal("running shell without exit_code must not arm")
	}

	recordShellOnFailure(ctx, &trpctool.AfterToolArgs{
		ToolName: "shell_exec",
		Result:   map[string]any{"exit_code": 1},
	})
	if !shellOnFailureArmed(ctx) {
		t.Fatal("non-zero exit must arm")
	}

	recordShellOnFailure(ctx, &trpctool.AfterToolArgs{
		ToolName: "exec_command",
		Result:   map[string]any{"exit_code": 0},
	})
	if shellOnFailureArmed(ctx) {
		t.Fatal("zero exit must clear")
	}

	recordShellOnFailure(ctx, &trpctool.AfterToolArgs{
		ToolName: "shell_exec",
		Error:    errors.New("start failed"),
	})
	if !shellOnFailureArmed(ctx) {
		t.Fatal("tool error must arm")
	}
}

func TestToolConfirmGate_ShellOnFailureConfirmsSafe(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	inv := &trpcagent.Invocation{}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	inv.SetState(shellOnFailureStateKey, true)

	d := g.decide(ctx, "sess-1", "agent-1", "shell_exec", []byte(`{"command":"go test ./..."}`))
	if !d.needsConfirm || d.reason != confirmReasonShellOnFailure {
		t.Fatalf("on-failure safe decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonShellOnFailure)
	}

	g.sessionGrants.GrantSession("sess-1", "agent-1", "shell_exec")
	d = g.decide(ctx, "sess-1", "agent-1", "shell_exec", []byte(`{"command":"go test ./..."}`))
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("granted on-failure decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantSession)
	}
}

func TestMCPDegradeToolCount_CodingWithoutAllow(t *testing.T) {
	t.Parallel()
	if got := mcpDegradeToolCount(&shardPlan{toolsProfile: "coding"}); got != 1 {
		t.Fatalf("coding without mcp: allow = %d, want 1", got)
	}
	if got := mcpDegradeToolCount(&shardPlan{toolsProfile: "coding", mcpAllowExplicit: true}); got != 8 {
		t.Fatalf("coding with mcp: allow = %d, want 8", got)
	}
	if got := mcpDegradeToolCount(&shardPlan{toolsProfile: "full"}); got != 20 {
		t.Fatalf("full = %d, want 20", got)
	}
}

func TestToolsAllowHasMCPServer(t *testing.T) {
	t.Parallel()
	if toolsAllowHasMCPServer(nil) {
		t.Fatal("nil settings")
	}
	if toolsAllowHasMCPServer(&biz.AgentRuntimeSettings{ToolsAllowJSON: `["shell_exec"]`}) {
		t.Fatal("plain allow must not count as mcp: allow")
	}
	if !toolsAllowHasMCPServer(&biz.AgentRuntimeSettings{ToolsAllowJSON: `["mcp:github"]`}) {
		t.Fatal("mcp:github must count")
	}
}

func TestToolConfirmGate_ShellSafeWithoutFailureStillSkips(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec", []byte(`{"command":"go test ./..."}`))
	if d.needsConfirm || d.reason != confirmReasonShellSafe {
		t.Fatalf("unarmed safe decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonShellSafe)
	}
}
