package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestCollapseConsecutiveFailedToolResults_KeepsFirstStubsRest(t *testing.T) {
	fail := `{"result":{"ok":false,"httpStatus":400,"error":"empty command"}}`
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleUser, Content: "fix sw1"},
		{Role: trpcmodel.RoleAssistant, Content: "exec", ToolCalls: []trpcmodel.ToolCall{{ID: "1"}}},
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", ToolID: "1", Content: fail},
		{Role: trpcmodel.RoleAssistant, Content: "retry", ToolCalls: []trpcmodel.ToolCall{{ID: "2"}}},
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", ToolID: "2", Content: fail},
		{Role: trpcmodel.RoleAssistant, Content: "again", ToolCalls: []trpcmodel.ToolCall{{ID: "3"}}},
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", ToolID: "3", Content: fail},
	}
	got := collapseConsecutiveFailedToolResults(msgs)
	if got != 2 {
		t.Fatalf("collapsed=%d, want 2", got)
	}
	if !strings.Contains(msgs[2].Content, "empty command") {
		t.Fatalf("first failure must stay, got %q", msgs[2].Content)
	}
	if !strings.HasPrefix(msgs[4].Content, failedToolResultDedupStubPrefix) {
		t.Fatalf("2nd failure must be stubbed, got %q", msgs[4].Content)
	}
	if !strings.HasPrefix(msgs[6].Content, failedToolResultDedupStubPrefix) {
		t.Fatalf("3rd failure must be stubbed, got %q", msgs[6].Content)
	}
}

func TestCollapseConsecutiveFailedToolResults_DifferentClassKept(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", Content: `{"ok":false,"httpStatus":400,"error":"empty command"}`},
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", Content: `{"ok":false,"httpStatus":501,"error":"not implemented"}`},
	}
	if got := collapseConsecutiveFailedToolResults(msgs); got != 0 {
		t.Fatalf("different error class must not collapse, got %d", got)
	}
}

func TestCollapseConsecutiveFailedToolResults_SuccessBreaksStreak(t *testing.T) {
	fail := `Error: boom`
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleTool, ToolName: "x", Content: fail},
		{Role: trpcmodel.RoleTool, ToolName: "x", Content: `{"ok":true,"output":"up"}`},
		{Role: trpcmodel.RoleTool, ToolName: "x", Content: fail},
	}
	if got := collapseConsecutiveFailedToolResults(msgs); got != 0 {
		t.Fatalf("success in between must reset streak, got %d", got)
	}
}

func TestFailedToolResultDedupHook_MutatesRequest(t *testing.T) {
	hook := newFailedToolResultDedupBeforeHook(loggateway.NewNoop())
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	fail := `{"ok":false,"error":"empty command"}`
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", Content: fail},
		{Role: trpcmodel.RoleTool, ToolName: "gns3_exec", Content: fail},
	}}}
	if _, err := fn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if !strings.HasPrefix(args.Request.Messages[1].Content, failedToolResultDedupStubPrefix) {
		t.Fatalf("hook must stub duplicate, got %q", args.Request.Messages[1].Content)
	}
}
