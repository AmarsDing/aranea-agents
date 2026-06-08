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

// ConditionalBranch defines a conditional routing branch from a source agent.
type ConditionalBranch struct {
	// SourceAgent is the agent whose output determines the branch.
	SourceAgent string `json:"source_agent" jsonschema:"description=Agent key whose output determines the branch"`
	// ConditionFunc is the function reference for evaluating the branch condition.
	ConditionFunc string `json:"condition_func" jsonschema:"description=Condition function reference (e.g. check_result_type)"`
	// PathMap maps condition result values to destination agent keys.
	PathMap map[string]string `json:"path_map" jsonschema:"description=Map of condition result value to destination agent key"`
	// DefaultPath is the fallback destination if no condition matches.
	DefaultPath string `json:"default_path,omitempty" jsonschema:"description=Fallback destination agent key if no condition matches"`
}

// BuildOrchestrationGraphInput is the input for the build_orchestration_graph tool.
type BuildOrchestrationGraphInput struct {
	TaskDescription     string               `json:"task_description" jsonschema:"description=Overall task description"`
	Agents              []AgentAssignment    `json:"agents" jsonschema:"description=Agent assignments for the graph"`
	Mode                string               `json:"mode" jsonschema:"description=Graph mode: parallel|sequential|hybrid|coordinator"`
	ConditionalBranches []ConditionalBranch  `json:"conditional_branches,omitempty" jsonschema:"description=Conditional routing branches (optional)"`
}

// BuildOrchestrationGraphOutput is the output for the build_orchestration_graph tool.
type BuildOrchestrationGraphOutput struct {
	GraphBuildConfig  biz.GraphBuildConfig `json:"graph_build_config"`
	GraphExecutionID  string               `json:"graph_execution_id,omitempty"`
	NodeCount         int                  `json:"node_count"`
	EdgeCount         int                  `json:"edge_count"`
	VerificationNodes []string             `json:"verification_nodes"`
}

// GraphBuilderPort defines the interface for executing built graphs.
type GraphBuilderPort interface {
	BuildAndExecute(ctx context.Context, config biz.GraphBuildConfig, sessionID string) (executionID string, err error)
}

// NewBuildOrchestrationGraphTool creates the build_orchestration_graph tool.
// If builder is nil, the tool only generates the graph config without executing it.
func NewBuildOrchestrationGraphTool(builder GraphBuilderPort) *trpcfunction.FunctionTool[BuildOrchestrationGraphInput, BuildOrchestrationGraphOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input BuildOrchestrationGraphInput) (BuildOrchestrationGraphOutput, error) {
			if len(input.Agents) == 0 {
				return BuildOrchestrationGraphOutput{}, kerrors.BadRequest("ORCHESTRATOR", "at least one agent is required")
			}

			config := BuildGraphConfig(input)
			verificationNodes := injectVerificationNodes(&config, input.Mode)

			out := BuildOrchestrationGraphOutput{
				GraphBuildConfig:  config,
				NodeCount:         len(config.Nodes),
				EdgeCount:         len(config.Edges),
				VerificationNodes: verificationNodes,
			}

			// Only execute the graph if a builder implementation is available.
			if builder != nil {
				execID, err := builder.BuildAndExecute(ctx, config, "")
				if err != nil {
					return BuildOrchestrationGraphOutput{}, err
				}
				out.GraphExecutionID = execID
			}

			return out, nil
		},
		trpcfunction.WithName("build_orchestration_graph"),
		trpcfunction.WithDescription("Build a Graph DAG for complex multi-agent orchestration. Use when 4+ agents need parallel/conditional execution with verification gates."),
	)
}

// BuildGraphConfig generates a GraphBuildConfig from the input specification.
func BuildGraphConfig(input BuildOrchestrationGraphInput) biz.GraphBuildConfig {
	var nodes []biz.NodeDef
	var edges []biz.EdgeDef

	entryNode := "entry"
	nodes = append(nodes, biz.NodeDef{ID: entryNode, Type: biz.NodeTypeFunction})
	agentKeys := buildAgentNodes(&nodes, input.Agents)
	buildDependencyEdges(&edges, entryNode, input.Agents, agentKeys)

	if hasCycle(nodes, edges) {
		buildSequentialChainEdges(&edges, entryNode, input.Agents)
	}

	mergeNode := "merge_results"
	nodes = append(nodes, biz.NodeDef{ID: mergeNode, Type: biz.NodeTypeFunction})
	buildMergeEdges(&edges, input.Agents, mergeNode)

	finishNode := "finish"
	nodes = append(nodes, biz.NodeDef{ID: finishNode, Type: biz.NodeTypeFunction})
	edges = append(edges, biz.EdgeDef{From: mergeNode, To: finishNode})

	// Build conditional edges from ConditionalBranches.
	var conditionalEdges []biz.ConditionalEdgeDef
	buildConditionalEdges(&conditionalEdges, &edges, input.ConditionalBranches, agentKeys, mergeNode)

	return biz.GraphBuildConfig{
		Nodes:            nodes,
		Edges:            edges,
		ConditionalEdges: conditionalEdges,
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

// buildAgentNodes creates agent nodes and returns the set of valid agent keys.
func buildAgentNodes(nodes *[]biz.NodeDef, agents []AgentAssignment) map[string]bool {
	agentKeys := make(map[string]bool, len(agents))
	for _, a := range agents {
		agentKeys[a.AgentKey] = true
		*nodes = append(*nodes, biz.NodeDef{
			ID:          a.AgentKey,
			Type:        biz.NodeTypeAgent,
			AgentName:   a.AgentKey,
			Instruction: a.SubTask,
		})
	}
	return agentKeys
}

// buildDependencyEdges creates edges based on agent dependencies.
// Dangling dependencies (referencing non-existent agents) are skipped.
func buildDependencyEdges(edges *[]biz.EdgeDef, entryNode string, agents []AgentAssignment, agentKeys map[string]bool) {
	for _, a := range agents {
		if len(a.DependsOn) == 0 {
			*edges = append(*edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
			continue
		}
		hasValidDep := false
		for _, dep := range a.DependsOn {
			if !agentKeys[dep] {
				continue
			}
			*edges = append(*edges, biz.EdgeDef{From: dep, To: a.AgentKey})
			hasValidDep = true
		}
		if !hasValidDep {
			*edges = append(*edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
		}
	}
}

// buildSequentialChainEdges creates a sequential chain as fallback when cycles are detected.
func buildSequentialChainEdges(edges *[]biz.EdgeDef, entryNode string, agents []AgentAssignment) {
	*edges = (*edges)[:0]
	for i, a := range agents {
		if i == 0 {
			*edges = append(*edges, biz.EdgeDef{From: entryNode, To: a.AgentKey})
		} else {
			*edges = append(*edges, biz.EdgeDef{From: agents[i-1].AgentKey, To: a.AgentKey})
		}
	}
}

// buildMergeEdges connects leaf agent nodes (those not depended on by others) to the merge node.
func buildMergeEdges(edges *[]biz.EdgeDef, agents []AgentAssignment, mergeNode string) {
	for _, a := range agents {
		if isDependedOn(agents, a.AgentKey) {
			continue
		}
		*edges = append(*edges, biz.EdgeDef{From: a.AgentKey, To: mergeNode})
	}
}

// isDependedOn checks if any agent depends on the given agent key.
func isDependedOn(agents []AgentAssignment, agentKey string) bool {
	for _, other := range agents {
		for _, dep := range other.DependsOn {
			if dep == agentKey {
				return true
			}
		}
	}
	return false
}

// hasCycle detects cycles in the graph using DFS.
func hasCycle(nodes []biz.NodeDef, edges []biz.EdgeDef) bool {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully processed
	)
	color := make(map[string]int, len(nodes))
	for _, n := range nodes {
		color[n.ID] = white
	}
	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		color[nodeID] = gray
		for _, next := range adj[nodeID] {
			if color[next] == gray {
				return true // back edge → cycle
			}
			if color[next] == white && dfs(next) {
				return true
			}
		}
		color[nodeID] = black
		return false
	}
	for _, n := range nodes {
		if color[n.ID] == white && dfs(n.ID) {
			return true
		}
	}
	return false
}

// buildConditionalEdges generates ConditionalEdgeDef entries from ConditionalBranch specifications.
// It also removes any static edges from the source agent to its conditional destinations,
// since the conditional edge replaces them with dynamic routing.
func buildConditionalEdges(
	condEdges *[]biz.ConditionalEdgeDef,
	edges *[]biz.EdgeDef,
	branches []ConditionalBranch,
	agentKeys map[string]bool,
	mergeNode string,
) {
	for _, cb := range branches {
		if !agentKeys[cb.SourceAgent] || cb.ConditionFunc == "" || len(cb.PathMap) == 0 {
			continue
		}

		// Build the conditional edge definition.
		pathMap := make(map[string]string, len(cb.PathMap))
		condDestinations := make(map[string]bool, len(cb.PathMap)+1)
		for result, dest := range cb.PathMap {
			if agentKeys[dest] {
				pathMap[result] = dest
				condDestinations[dest] = true
			}
		}
		if cb.DefaultPath != "" && agentKeys[cb.DefaultPath] {
			pathMap["__default__"] = cb.DefaultPath
			condDestinations[cb.DefaultPath] = true
		}

		if len(pathMap) == 0 {
			continue
		}

		*condEdges = append(*condEdges, biz.ConditionalEdgeDef{
			From:        cb.SourceAgent,
			CondFuncRef: cb.ConditionFunc,
			PathMap:     pathMap,
		})

		// Remove static edges from source to conditional destinations,
		// since the conditional edge handles routing dynamically.
		var filteredEdges []biz.EdgeDef
		for _, e := range *edges {
			if e.From == cb.SourceAgent && condDestinations[e.To] {
				continue // skip static edge to conditional destination
			}
			filteredEdges = append(filteredEdges, e)
		}
		*edges = filteredEdges
	}
}
