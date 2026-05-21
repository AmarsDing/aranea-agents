package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/mcp/classify"
)

func TestClassifyToolInvocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	deps := TRPCBuilderDeps{}

	mcp, skill := classifyToolInvocation(ctx, "mcp_call", nil, deps)
	if !mcp || skill {
		t.Fatalf("mcp_call: got mcp=%v skill=%v", mcp, skill)
	}

	mcp, skill = classifyToolInvocation(ctx, "use_skill", nil, deps)
	if mcp || !skill {
		t.Fatalf("use_skill: got mcp=%v skill=%v", mcp, skill)
	}

	mcp, skill = classifyToolInvocation(ctx, "mcp_demo__fetch", nil, deps)
	if !mcp || skill {
		t.Fatalf("prefixed: got mcp=%v skill=%v", mcp, skill)
	}
}

func TestClassifyToolInvocationUnknownKey(t *testing.T) {
	t.Parallel()
	mcp, skill := classifyToolInvocation(context.Background(), "read_file", nil, TRPCBuilderDeps{})
	if mcp || skill {
		t.Fatalf("read_file: got mcp=%v skill=%v", mcp, skill)
	}
}

func TestClassifyToolInvocationMCPResultMeta(t *testing.T) {
	t.Parallel()
	if !classify.IsMCPToolInvocation("remote_search", classifyTestMCPResult{meta: map[string]any{"k": "v"}}) {
		t.Fatal("expected MCP via result meta")
	}
}

type classifyTestMCPResult struct{ meta map[string]any }

func (c classifyTestMCPResult) GetMeta() map[string]any { return c.meta }
