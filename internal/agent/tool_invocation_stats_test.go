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

// TestClassifyToolInvocationMCPResultNilMeta pins the fix that relaxed the
// metaGetter nil check. Playwright MCP tools (playwright_browser_*) return no
// Meta, so the previous `mg.GetMeta() != nil` guard made them fall through to
// the non-MCP path — leaving mcpCount=0 and source="trpc" in tool_invocations.
// The result type assertion alone is a reliable MCP marker because only
// *mcp.Tool.mcpToolResult implements GetMeta() in trpc-agent-go.
func TestClassifyToolInvocationMCPResultNilMeta(t *testing.T) {
	t.Parallel()
	// Direct classifier check: nil-meta MCP result is MCP.
	if !classify.IsMCPToolInvocation("playwright_browser_navigate", classifyTestMCPResult{meta: nil}) {
		t.Fatal("expected MCP via nil-meta result type marker")
	}
	// Agent-layer wrapper must agree so that recordToolInvocationWrite sets
	// write.Source = biz.ToolInvocationSourceMCP and bumps mcpDelta.
	mcp, skill := classifyToolInvocation(
		context.Background(),
		"playwright_browser_navigate",
		classifyTestMCPResult{meta: nil},
		TRPCBuilderDeps{},
	)
	if !mcp {
		t.Fatal("classifyToolInvocation: nil-meta MCP result should classify as MCP")
	}
	if skill {
		t.Fatal("classifyToolInvocation: nil-meta MCP result should not classify as skill")
	}
}

type classifyTestMCPResult struct{ meta map[string]any }

func (c classifyTestMCPResult) GetMeta() map[string]any { return c.meta }
