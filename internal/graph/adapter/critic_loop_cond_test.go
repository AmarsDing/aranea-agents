package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestCriticLoopCondFunc_ToolCallApprove(t *testing.T) {
	fn := criticLoopCondFunc(0.8, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"approve","score":0.9,"reason":"looks good"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved, got %s", result)
	}
}

func TestCriticLoopCondFunc_ToolCallRetry(t *testing.T) {
	fn := criticLoopCondFunc(0.8, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"retry","reason":"needs improvement"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry, got %s", result)
	}
}

func TestCriticLoopCondFunc_ToolCallScoreAboveThreshold(t *testing.T) {
	fn := criticLoopCondFunc(0.7, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"retry","score":0.8}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (score >= threshold), got %s", result)
	}
}

func TestCriticLoopCondFunc_FallbackStringApproved(t *testing.T) {
	fn := criticLoopCondFunc(0.8, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: "I have reviewed the work and it is approved."},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (string fallback), got %s", result)
	}
}

func TestCriticLoopCondFunc_FallbackStringScore(t *testing.T) {
	fn := criticLoopCondFunc(0.7, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: `{"score": 0.8}`},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (score fallback), got %s", result)
	}
}

func TestCriticLoopCondFunc_NoMessages(t *testing.T) {
	fn := criticLoopCondFunc(0.8, loggateway.NewNoop())
	result, err := fn(context.Background(), trpcgraph.State{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (no messages), got %s", result)
	}
}

func TestCriticLoopCondFunc_OtherToolCallIgnored(t *testing.T) {
	fn := criticLoopCondFunc(0.8, loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: "needs work", ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      "some_other_tool",
					Arguments: []byte(`{"action":"approve"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (other tool call ignored, no 'approved' in content), got %s", result)
	}
}

func TestExtractScore(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{`{"score": 0.85}`, 0.85},
		{`[{"score": 0.9}]`, 0.9},
		{`no score here`, 0},
		{``, 0},
	}
	for _, tt := range tests {
		got := biz.ExtractScore(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractScore(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}
