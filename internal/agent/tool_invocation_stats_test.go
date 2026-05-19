package agent

import (
	"context"
	"testing"

)

func TestClassifyToolInvocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	deps := TRPCBuilderDeps{}

	mcp, skill := classifyToolInvocation(ctx, "mcp_call", deps)
	if !mcp || skill {
		t.Fatalf("mcp_call: got mcp=%v skill=%v", mcp, skill)
	}

	mcp, skill = classifyToolInvocation(ctx, "use_skill", deps)
	if mcp || !skill {
		t.Fatalf("use_skill: got mcp=%v skill=%v", mcp, skill)
	}
}

func TestClassifyToolInvocationUnknownKey(t *testing.T) {
	t.Parallel()
	mcp, skill := classifyToolInvocation(context.Background(), "read_file", TRPCBuilderDeps{})
	if mcp || skill {
		t.Fatalf("read_file: got mcp=%v skill=%v", mcp, skill)
	}
}
