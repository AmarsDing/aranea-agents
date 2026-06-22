package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func callTool(t *testing.T, tool *trpcfunction.FunctionTool[BuildOrchestrationGraphInput, BuildOrchestrationGraphOutput], input BuildOrchestrationGraphInput) BuildOrchestrationGraphOutput {
	t.Helper()
	args, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	out, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("tool.Call: %v", err)
	}
	result, ok := out.(BuildOrchestrationGraphOutput)
	if !ok {
		t.Fatalf("expected BuildOrchestrationGraphOutput, got %T", out)
	}
	return result
}

func TestNewBuildOrchestrationGraphTool_nilBuilderAddsWarning(t *testing.T) {
	tool := NewBuildOrchestrationGraphTool(nil)
	out := callTool(t, tool, BuildOrchestrationGraphInput{
		TaskDescription: "test task",
		Agents: []AgentAssignment{
			{AgentKey: "agent_a", Role: "worker", SubTask: "do A"},
			{AgentKey: "agent_b", Role: "worker", SubTask: "do B"},
		},
		Mode: "parallel",
	})
	if out.GraphExecutionID != "" {
		t.Errorf("GraphExecutionID should be empty when builder is nil, got %q", out.GraphExecutionID)
	}
	foundWarning := false
	for _, w := range out.Warnings {
		if strings.Contains(w, "graph builder not configured") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected warning about nil builder, got %v", out.Warnings)
	}
}

func TestNewBuildOrchestrationGraphTool_withBuilderExecutes(t *testing.T) {
	stub := &stubGraphBuilder{execID: "exec-123"}
	tool := NewBuildOrchestrationGraphTool(stub)
	out := callTool(t, tool, BuildOrchestrationGraphInput{
		TaskDescription: "test task",
		Agents: []AgentAssignment{
			{AgentKey: "agent_a", Role: "worker", SubTask: "do A"},
		},
		Mode: "sequential",
	})
	if out.GraphExecutionID != "exec-123" {
		t.Errorf("GraphExecutionID = %q, want %q", out.GraphExecutionID, "exec-123")
	}
	if !stub.called {
		t.Error("builder.BuildAndExecute should have been called")
	}
}

func TestNewBuildOrchestrationGraphTool_emptyAgentsReturnsError(t *testing.T) {
	tool := NewBuildOrchestrationGraphTool(nil)
	args, _ := json.Marshal(BuildOrchestrationGraphInput{
		TaskDescription: "test task",
		Agents:          []AgentAssignment{},
		Mode:            "parallel",
	})
	_, err := tool.Call(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for empty agents, got nil")
	}
}

type stubGraphBuilder struct {
	execID string
	called bool
}

func (s *stubGraphBuilder) BuildAndExecute(ctx context.Context, config biz.GraphBuildConfig, sessionID string) (string, error) {
	s.called = true
	return s.execID, nil
}
