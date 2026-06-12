package a2ui

import (
	"context"

	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
)

type PlanToGraphConverter struct{}

func NewPlanToGraphConverter() *PlanToGraphConverter {
	return &PlanToGraphConverter{}
}

func (c *PlanToGraphConverter) Convert(ctx context.Context, plan *Plan) (*biz.GraphBuildConfig, error) {
	if len(plan.Steps) == 0 {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "plan has no steps")
	}

	cfg := &biz.GraphBuildConfig{
		StateFields: []biz.StateFieldDef{
			{Name: "goal", Type: "string", Reducer: biz.ReducerDefault},
			{Name: "current_step", Type: "string", Reducer: biz.ReducerDefault},
			{Name: "step_results", Type: "map", Reducer: biz.ReducerMerge},
			{Name: "step_statuses", Type: "map", Reducer: biz.ReducerMerge},
		},
		ExecutionEngine: biz.EngineBSP,
	}

	stepIDs := make([]string, 0, len(plan.Steps))
	nodeDefs := make(map[string]biz.NodeDef)

	for _, step := range plan.Steps {
		stepIDs = append(stepIDs, step.ID)
		nd := biz.NodeDef{
			ID:          step.ID,
			FuncRef:     step.ID,
			Type:        "agent",
			Description: step.Description,
			AgentName:   step.AgentName,
		}
		if len(step.Tools) > 0 {
			nd.ToolNames = step.Tools
		}
		nodeDefs[step.ID] = nd
		cfg.Nodes = append(cfg.Nodes, nd)
	}

	entryStep := plan.Steps[0]
	cfg.EntryPoint = entryStep.ID

	depGraph := buildDependencyGraph(plan)
	order, err := topologicalSort(depGraph, stepIDs)
	if err != nil {
		return nil, err // already apierror.BadRequest
	}

	if len(order) > 0 {
		cfg.FinishPoint = order[len(order)-1]
	}

	for i, stepID := range order {
		deps := depGraph[stepID]
		if len(deps) == 0 && i == 0 {
			continue
		}
		if len(deps) == 0 {
			prev := order[i-1]
			cfg.Edges = append(cfg.Edges, biz.EdgeDef{From: prev, To: stepID})
			continue
		}
		for _, dep := range deps {
			cfg.Edges = append(cfg.Edges, biz.EdgeDef{From: dep, To: stepID})
		}
	}

	return cfg, nil
}

func buildDependencyGraph(plan *Plan) map[string][]string {
	g := make(map[string][]string, len(plan.Steps))
	for _, step := range plan.Steps {
		g[step.ID] = step.DependsOn
	}
	if plan.Dependencies != nil {
		for id, deps := range plan.Dependencies {
			g[id] = append(g[id], deps...)
		}
	}
	return g
}

func topologicalSort(depGraph map[string][]string, allIDs []string) ([]string, error) {
	// Build the full set of node IDs: explicit steps + any IDs that appear
	// only in Dependencies but not in Steps (ghost nodes from plan.Dependencies).
	allNodeSet := make(map[string]struct{}, len(allIDs))
	for _, id := range allIDs {
		allNodeSet[id] = struct{}{}
	}
	for id := range depGraph {
		if _, ok := allNodeSet[id]; !ok {
			allNodeSet[id] = struct{}{}
		}
	}

	inDegree := make(map[string]int, len(allNodeSet))
	for id := range allNodeSet {
		inDegree[id] = 0
	}
	for id, deps := range depGraph {
		inDegree[id] = len(deps)
	}

	var queue []string
	for id := range allNodeSet {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		for otherID, deps := range depGraph {
			for _, d := range deps {
				if d == id {
					inDegree[otherID]--
					if inDegree[otherID] == 0 {
						queue = append(queue, otherID)
					}
				}
			}
		}
	}

	if len(order) != len(allNodeSet) {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "cycle detected in UI node dependencies")
	}
	return order, nil
}
