package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestPermissionGuard_DenyToolsOnly(t *testing.T) {
	p := NewPermissionGuardPlugin(
		biz.Plugin{Key: "permission_guard", ConfigJSON: `{"deny_tools":["execute_sql"]}`},
		nil, nil, nil, loggateway.NewNoop(),
	)

	res, err := p.beforeTool(context.Background(), &trpctool.BeforeToolArgs{ToolName: "delete_file"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CustomResult != nil {
		t.Fatalf("non-denied tool should pass through permission_guard, got %#v", res.CustomResult)
	}

	res, err = p.beforeTool(context.Background(), &trpctool.BeforeToolArgs{ToolName: "execute_sql"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CustomResult == nil {
		t.Fatal("deny_tools should block")
	}
}

func TestPermissionGuard_AgentAllowlistByPlatformID(t *testing.T) {
	resolver := func(_ context.Context, agentKey string) string {
		if agentKey == "my-agent" {
			return "agent-uuid-1"
		}
		return ""
	}
	p := NewPermissionGuardPlugin(
		biz.Plugin{Key: "permission_guard", ConfigJSON: `{"agent_allowlist":["agent-uuid-1"],"deny_tools":["execute_sql"]}`},
		nil, nil, resolver, loggateway.NewNoop(),
	)
	inv := trpcagent.NewInvocation()
	inv.AgentName = "my-agent"
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	res, err := p.beforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "execute_sql"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CustomResult == nil {
		t.Fatal("deny_tools should block when agent matches allowlist by platform id")
	}
}
