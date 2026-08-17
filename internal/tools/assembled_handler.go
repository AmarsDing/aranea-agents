package tools

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// NewAssembledToolHandler dispatches ParallelToolExecutor calls through the
// same CallableTool.Call used by the LLM path (decorator + normalize + locks).
// Prefer BatchExecuteAssembledTools, which clears IsolationStrategy and does
// not copy the Wire worktree isolator. Callers that use this handler directly
// must leave ToolCall.IsolationStrategy empty so the batch executor does not
// wrap a second git worktree around tools that already isolate.
func NewAssembledToolHandler(flat []trpctool.Tool, sets []trpctool.ToolSet) ToolHandler {
	return func(ctx context.Context, call ToolCall) ToolResult {
		ct := lookupAssembledCallable(ctx, flat, sets, call.Name)
		if ct == nil {
			return ToolResult{
				CallID: call.ID,
				Name:   call.Name,
				Error:  apierror.NotFound(apierror.DomainTool, "tool not assembled: "+call.Name).Error(),
			}
		}
		out, err := ct.Call(ctx, call.Arguments)
		if err != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		raw, marshalErr := json.Marshal(out)
		if marshalErr != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: ""}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: string(raw)}
	}
}

func lookupAssembledCallable(ctx context.Context, flat []trpctool.Tool, sets []trpctool.ToolSet, name string) trpctool.CallableTool {
	if ct := matchAssembledCallable(flat, name); ct != nil {
		return ct
	}
	for _, set := range sets {
		if set == nil {
			continue
		}
		if ct := matchAssembledCallable(set.Tools(ctx), name); ct != nil {
			return ct
		}
	}
	return nil
}

func matchAssembledCallable(list []trpctool.Tool, want string) trpctool.CallableTool {
	want = strings.TrimSpace(want)
	canon := canonicalRuntimeName(want)
	for _, t := range list {
		if t == nil {
			continue
		}
		d := t.Declaration()
		if d == nil {
			continue
		}
		got := strings.TrimSpace(d.Name)
		if got != want && canonicalRuntimeName(got) != canon {
			continue
		}
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			continue
		}
		return ct
	}
	return nil
}
