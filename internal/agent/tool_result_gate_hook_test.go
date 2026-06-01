package agent

import (
	"testing"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestEstimateTurnNumber(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleUser},
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleUser},
		{Role: trpcmodel.RoleTool},
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleUser},
	}
	n := estimateTurnNumber(msgs)
	if n != 3 {
		t.Fatalf("estimateTurnNumber = %d, want 3", n)
	}
}

func TestEstimateTurnNumber_NoUserMessages(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleTool},
	}
	n := estimateTurnNumber(msgs)
	if n != 0 {
		t.Fatalf("estimateTurnNumber = %d, want 0", n)
	}
}

func TestEstimateTurnNumber_Empty(t *testing.T) {
	n := estimateTurnNumber(nil)
	if n != 0 {
		t.Fatalf("estimateTurnNumber = %d, want 0", n)
	}
}

func TestExtractTextContent_PlainContent(t *testing.T) {
	msg := &trpcmodel.Message{Content: "hello world"}
	got := extractTextContent(msg)
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestExtractTextContent_ContentParts(t *testing.T) {
	text := "from parts"
	msg := &trpcmodel.Message{
		ContentParts: []trpcmodel.ContentPart{
			{Type: trpcmodel.ContentTypeText, Text: &text},
		},
	}
	got := extractTextContent(msg)
	if got != "from parts" {
		t.Fatalf("got %q, want %q", got, "from parts")
	}
}

func TestExtractTextContent_Empty(t *testing.T) {
	msg := &trpcmodel.Message{}
	got := extractTextContent(msg)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestToolResultGateEnabled_Default(t *testing.T) {
	if !toolResultGateEnabled(biz.Agent{}) {
		t.Fatal("default should be enabled when Settings is nil")
	}
}

func TestToolResultGateEnabled_WithSettings(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolResultGateEnabled: true}}
	if !toolResultGateEnabled(ag) {
		t.Fatal("explicitly enabled should return true")
	}
}

func TestToolResultGateEnabled_Disabled(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolResultGateEnabled: false}}
	if toolResultGateEnabled(ag) {
		t.Fatal("explicitly disabled should return false")
	}
}
