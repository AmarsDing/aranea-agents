package orchestrator

import (
	"context"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// AgentAssignment defines an agent's role in the orchestration graph.
type AgentAssignment struct {
	AgentKey  string   `json:"agent_key" jsonschema:"description=Agent key"`
	Role      string   `json:"role" jsonschema:"description=Agent role in the task"`
	SubTask   string   `json:"sub_task" jsonschema:"description=Sub-task description for this agent"`
	DependsOn []string `json:"depends_on" jsonschema:"description=Agent keys this agent depends on"`
}

// BuildOrchestrationGraphInput is the input for the build_orchestration_graph tool.
type BuildOrchestrationGraphInput struct {
	TaskDescription string            `json:"task_description" jsonschema:"description=Overall task description"`
	Agents          []AgentAssignment `json:"agents" jsonschema:"description=Agent assignments for the graph"`
	Mode            string            `json:"mode" jsonschema:"description=Graph mode: parallel|sequential|hybrid|coordinator"`
}

// BuildOrchestrationGraphOutput is the output for the build_orchestration_graph tool.
type BuildOrchestrationGraphOutput struct {
	GraphBuildConfig  biz.GraphBuildConfig `json:"graph_build_config"`
	NodeCount         int                  `json:"node_count"`
	EdgeCount         int                  `json:"edge_count"`
	VerificationNodes []string             `json:"verification_nodes"`
}

// GraphBuilderPort defines the interface for executing built graphs.
type GraphBuilderPort interface {
	BuildAndExecute(ctx context.Context, config biz.GraphBuildConfig, sessionID string) (executionID string, err error)
}

// NewBuildOrchestrationGraphTool creates the build_orchestration_graph tool.
func NewBuildOrchestrationGraphTool(builder GraphBuilderPort) *trpcfunction.FunctionTool[BuildOrchestrationGraphInput, BuildOrchestrationGraphOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input BuildOrchestrationGraphInput) (BuildOrchestrationGraphOutput, error) {
			if len(input.Agents) == 0 {
				return BuildOrchestrationGraphOutput{}, kerrors.BadRequest("ORCHESTRATOR", "at least one agent is required")
			}

			config := BuildGraphConfig(input)
			verificationNodes := injectVerificationNodes(&config, input.Mode)

			return BuildOrchestrationGraphOutput{
				GraphBuildConfig:  config,
				NodeCount:         len(config.Nodes),
				EdgeCount:         len(config.Edges),
				VerificationNodes: verificationNodes,
			}, nil
		},
		trpcfunction.WithName("build_orchestration_graph"),
		trpcfunction.WithDescription("Build a Graph DAG for complex multi-agent orchestration. Use when 4+ agents need parallel/conditional execution with verification gates."),
	)
}

// BuildGraphConfig generates a GraphBuildConfig from the input specification.
func BuildGraphConfig(input BuildOrchestrationGraphInput) biz.GraphBuildConfig {
	var nodes []biz.NodeDef
	var edges []biz.EdgeDef

	// 1. Entry node
	entryNode := "entry"
	nodes = append(nodes, biz.NodeDef{ID: entryNode, Type: biz.NodeTypeFunction})

	// 2. Agent nodes
	for _, a := range input.Agents {
		nodes = append(nodes, biz.NodeDef{
			ID:          a.AgentKey,
			Type:        biz.NodeTypeAgent,
			AgentName:   a.AgentKey,
			Instruction: a.SubTask,
		})
	}

	// 3. Edges based on dependencies
	for _, a := range input.Agents {
		if len(a.DependsOn) == 0 {
			edges = append(edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
		} else {
			for _, dep := range a.DependsOn {
				edges = append(edges, biz.EdgeDef{From: dep, To: a.AgentKey})
			}
		}
	}

	// 4. Merge node — all leaf agent nodes converge here
	mergeNode := "merge_results"
	nodes = append(nodes, biz.NodeDef{ID: mergeNode, Type: biz.NodeTypeFunction})
	for _, a := range input.Agents {
		isDependedOn := false
		for _, other := range input.Agents {
			for _, dep := range other.DependsOn {
				if dep == a.AgentKey {
					isDependedOn = true
					break
				}
			}
			if isDependedOn {
				break
			}
		}
		if !isDependedOn {
			edges = append(edges, biz.EdgeDef{From: a.AgentKey, To: mergeNode})
		}
	}

	// 5. Finish point
	finishNode := "finish"
	nodes = append(nodes, biz.NodeDef{ID: finishNode, Type: biz.NodeTypeFunction})
	edges = append(edges, biz.EdgeDef{From: mergeNode, To: finishNode})

	return biz.GraphBuildConfig{
		Nodes:            nodes,
		Edges:            edges,
		EntryPoint:       entryNode,
		FinishPoint:      finishNode,
		EnableCheckpoint: true,
		ExecutionEngine:  biz.EngineDAG,
		StateFields: []biz.StateFieldDef{
			{Name: "task_description", Reducer: biz.ReducerDefault},
			{Name: "agent_results", Reducer: biz.ReducerMerge},
		},
	}
}
