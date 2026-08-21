package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func runToolNotFoundHook(t *testing.T, req *trpcmodel.Request) *trpcmodel.Request {
	t.Helper()
	hook, ok := newToolNotFoundFeedbackBeforeHook(nil).(callbacks.BeforeModelHook)
	if !ok {
		t.Fatal("hook does not implement BeforeModelHook")
	}
	res, err := hook.HandleBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{Request: req})
	if err != nil {
		t.Fatalf("HandleBeforeModel error: %v", err)
	}
	if res == nil {
		t.Fatal("nil BeforeModelResult")
	}
	return req
}

func TestToolNotFoundFeedback_InjectsToolListOnLastError(t *testing.T) {
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			{Role: trpcmodel.RoleUser, Content: "查一下股票"},
			{Role: trpcmodel.RoleTool, ToolName: "hostexec_exec_command", Content: "tool execution error: executeToolCall: Error: tool not found: hostexec_exec_command"},
			{Role: trpcmodel.RoleAssistant, Content: "换个名字试试"},
			{Role: trpcmodel.RoleTool, ToolName: "exec_command", Content: "tool execution error: executeToolCall: Error: tool not found: exec_command"},
		},
		Tools: map[string]trpctool.Tool{
			"shell_exec":       nil,
			"plan_and_execute": nil,
			"tool_load":        nil,
		},
	}
	runToolNotFoundHook(t, req)

	first := req.Messages[1].Content
	if strings.Contains(first, toolNotFoundGuidanceTag) {
		t.Fatalf("earlier error message should not be rewritten, got: %s", first)
	}
	last := req.Messages[3].Content
	if !strings.Contains(last, toolNotFoundGuidanceTag) {
		t.Fatalf("last error message missing guidance, got: %s", last)
	}
	for _, name := range []string{"shell_exec", "plan_and_execute", "tool_load"} {
		if !strings.Contains(last, name) {
			t.Fatalf("guidance missing tool name %q, got: %s", name, last)
		}
	}
	if !strings.Contains(last, "exec_command") {
		t.Fatalf("original error text must be preserved, got: %s", last)
	}
}

func TestToolNotFoundFeedback_NoErrorMessageNoOp(t *testing.T) {
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			{Role: trpcmodel.RoleTool, ToolName: "datetime", Content: `{"date":"2026-08-21"}`},
		},
		Tools: map[string]trpctool.Tool{"datetime": nil},
	}
	runToolNotFoundHook(t, req)
	if strings.Contains(req.Messages[0].Content, toolNotFoundGuidanceTag) {
		t.Fatalf("successful tool result must not be rewritten, got: %s", req.Messages[0].Content)
	}
}

func TestToolNotFoundFeedback_IdempotentWithinSameRequest(t *testing.T) {
	msg := trpcmodel.Message{
		Role:    trpcmodel.RoleTool,
		Content: "tool execution error: executeToolCall: Error: tool not found: foo",
	}
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{msg},
		Tools:    map[string]trpctool.Tool{"shell_exec": nil},
	}
	runToolNotFoundHook(t, req)
	runToolNotFoundHook(t, req)
	if got := strings.Count(req.Messages[0].Content, toolNotFoundGuidanceTag); got != 1 {
		t.Fatalf("guidance appended %d times, want 1", got)
	}
}

func TestToolNotFoundFeedback_EmptyToolSurfaceNoOp(t *testing.T) {
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			{Role: trpcmodel.RoleTool, Content: "Error: tool not found: foo"},
		},
	}
	runToolNotFoundHook(t, req)
	if strings.Contains(req.Messages[0].Content, toolNotFoundGuidanceTag) {
		t.Fatalf("no tools available -> no guidance expected, got: %s", req.Messages[0].Content)
	}
}
