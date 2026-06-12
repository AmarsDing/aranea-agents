package agent

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// DAGToGraphCompiler converts TaskDAG + AllocationPlan into a Team Definition JSON
// that can be fed into the existing CompileToCompiledTeam pipeline.
// The output format matches buildSpiritTeamDefinitionJSON in spirit_team_usecase.go.
type DAGToGraphCompiler struct {
	lg loggateway.Logger
}

// NewDAGToGraphCompiler creates a new DAGToGraphCompiler.
func NewDAGToGraphCompiler(lg loggateway.Logger) *DAGToGraphCompiler {
	return &DAGToGraphCompiler{lg: lg}
}

// dagMember mirrors the member struct from buildSpiritTeamDefinitionJSON.
type dagMember struct {
	AgentKey string `json:"agent_key"`
	Role     string `json:"role"`
	Enabled  *bool  `json:"enabled"`
}

// Compile converts a PlanTaskDAG and AllocationPlan into a Definition JSON string
// that can be fed into the existing CompileToCompiledTeam pipeline.
func (c *DAGToGraphCompiler) Compile(dag *biz.PlanTaskDAG, allocPlan *biz.AllocationPlan) (string, error) {
	if dag == nil || allocPlan == nil {
		return "{}", apierror.BadRequest(apierror.DomainSpirit, "dag and allocPlan must not be nil")
	}

	// Determine mode based on DAG structure.
	// If there are dependencies, use "coordinator" so a synthesizer node is added.
	// If all nodes are independent (no depends_on), use "parallel".
	mode := "coordinator"
	hasDependencies := false
	for _, node := range dag.Nodes {
		if len(node.DependsOn) > 0 {
			hasDependencies = true
			break
		}
	}
	if !hasDependencies && len(dag.Nodes) > 1 {
		mode = "parallel"
	}

	// Build members array from DAG nodes + allocations.
	members := make([]dagMember, 0, len(allocPlan.Allocations)+1)

	// Add synthesizer member (first member, always present for coordinator mode).
	if mode == "coordinator" {
		synthEnabled := true
		members = append(members, dagMember{
			AgentKey: "__spirit__",
			Role:     "synthesizer",
			Enabled:  &synthEnabled,
		})
	}

	// Build a lookup from subtask ID to SubTask for dependency resolution.
	subTaskByID := make(map[string]*biz.SubTask, len(dag.Nodes))
	for i := range dag.Nodes {
		subTaskByID[dag.Nodes[i].ID] = &dag.Nodes[i]
	}

	// Validate: detect cycles in the DAG before proceeding.
	if err := validateDAGNoCycle(dag); err != nil {
		return "{}", err // already apierror.BadRequest
	}

	// Add worker members from allocations.
	for i := range allocPlan.Allocations {
		alloc := &allocPlan.Allocations[i]
		enabled := true
		member := dagMember{
			AgentKey: strings.TrimSpace(alloc.AssignedKey),
			Role:     "worker",
			Enabled:  &enabled,
		}
		// If the agent key is empty, log a warning and skip.
		if member.AgentKey == "" {
			c.lg.Warn("DAGToGraphCompiler: allocation has empty assigned_key, skipping",
				loggateway.StepID("spirit.orchestrator.graph_build"),
				loggateway.Str("sub_task_id", alloc.SubTaskID),
				loggateway.Str("sub_task_name", alloc.SubTaskName),
			)
			continue
		}
		members = append(members, member)
	}

	// Build the definition matching buildSpiritTeamDefinitionJSON format.
	def := map[string]any{
		"version":            2,
		"mode":               mode,
		"runtime_engine":     "graph",
		"team_graph_runtime": true,
		"members":            members,
		"max_concurrency":    2,
		"timeout_seconds":    600,
	}

	out, err := json.Marshal(def)
	if err != nil {
		return "{}", apierror.Internal(apierror.DomainSpirit, "marshal definition").WithCause(err)
	}
	return string(out), nil
}

// validateDAGNoCycle checks that the DAG has no cycles using topological sort.
func validateDAGNoCycle(dag *biz.PlanTaskDAG) error {
	if dag == nil || len(dag.Nodes) == 0 {
		return nil
	}
	inDegree := make(map[string]int, len(dag.Nodes))
	for _, n := range dag.Nodes {
		inDegree[n.ID] = len(n.DependsOn)
	}
	var queue []string
	for _, n := range dag.Nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, n := range dag.Nodes {
			for _, dep := range n.DependsOn {
				if dep == id {
					inDegree[n.ID]--
					if inDegree[n.ID] == 0 {
						queue = append(queue, n.ID)
					}
				}
			}
		}
	}
	if visited != len(dag.Nodes) {
		return apierror.BadRequest(apierror.DomainSpirit, "DAG cycle detected: not all nodes reachable (%d/%d)", visited, len(dag.Nodes))
	}
	return nil
}
