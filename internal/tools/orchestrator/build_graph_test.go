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

// TestBuildGraphConfig_FunctionNodesHaveFuncRef 回归：2026-09-03 108 实证
// entry/merge_results/finish/verify_* 这类结构/门节点是 function 类型，node_wiring
// 要求 function 节点必须带 Func 或 SkipNodeFuncRef，否则图构建直接 BAD_REQUEST。
// 所有模式（含会注入验证节点的 parallel/hybrid/coordinator）都必须满足。
func TestBuildGraphConfig_FunctionNodesHaveFuncRef(t *testing.T) {
	for _, mode := range []string{"sequential", "parallel", "hybrid", "coordinator", "dag", ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg, _ := BuildGraphConfig(BuildOrchestrationGraphInput{
				TaskDescription: "t",
				Agents: []AgentAssignment{
					{AgentKey: "a", SubTask: "A"},
					{AgentKey: "b", SubTask: "B", DependsOn: []string{"a"}},
				},
				Mode: mode,
			})
			injectVerificationNodes(&cfg, mode)
			for _, n := range cfg.Nodes {
				if n.Type == biz.NodeTypeFunction && strings.TrimSpace(n.FuncRef) == "" {
					t.Errorf("mode=%q node %q is function type without FuncRef", mode, n.ID)
				}
			}
		})
	}
}

// TestBuildGraphConfig_StateFieldsHaveType 回归：2026-09-03 108 实证（P-A 修复
// FuncRef 后暴露的下一层卡口）——builder 对空 Type 不补默认，框架 Compile 校验
// 报 "field X has nil type"，图构建失败。所有声明的 StateField 必须带可解析 Type。
func TestBuildGraphConfig_StateFieldsHaveType(t *testing.T) {
	cfg, _ := BuildGraphConfig(BuildOrchestrationGraphInput{
		TaskDescription: "t",
		Agents:          []AgentAssignment{{AgentKey: "a", SubTask: "A"}},
		Mode:            "parallel",
	})
	if len(cfg.StateFields) == 0 {
		t.Fatal("expected StateFields to be declared")
	}
	for _, sf := range cfg.StateFields {
		if strings.TrimSpace(sf.Type) == "" {
			t.Errorf("state field %q has empty Type (compile would fail: field has nil type)", sf.Name)
		}
	}
}
