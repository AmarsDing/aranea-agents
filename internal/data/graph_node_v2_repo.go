package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphnodev2"
	"aranea-agents/pkg/loggateway"
)

// graphNodeV2Repo implements biz.GraphNodeV2Repo.
// Stability:evolving
//
// GraphNode.DependsOn is an in-memory field only (derived from plan_steps.depends_on);
// it is NOT persisted to the graph_nodes_v2 table.
type graphNodeV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.GraphNodeV2Repo = (*graphNodeV2Repo)(nil)

// NewGraphNodeV2Repo creates a new GraphNodeV2Repo.
// Logger is preset with domain "GRAPH_NODE_V2" per loggateway convention.
func NewGraphNodeV2Repo(d *Data, lg loggateway.Logger) biz.GraphNodeV2Repo {
	return &graphNodeV2Repo{data: d, lg: lg.With(loggateway.Domain("GRAPH_NODE_V2"))}
}

// GetGraphNode returns the GraphNode by ID.
// DependsOn is intentionally left empty (in-memory only, derived from PlanStep).
func (r *graphNodeV2Repo) GetGraphNode(ctx context.Context, id string) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).GraphNodeV2.Get(ctx, id)
	if err != nil {
		return biz.GraphNode{}, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodeV2ToBiz(row), nil
}

// ListGraphNodesByStage returns all graph nodes for the given graph stage, ordered by id asc.
// DependsOn is intentionally left empty (in-memory only, derived from PlanStep).
func (r *graphNodeV2Repo) ListGraphNodesByStage(ctx context.Context, graphStageID string) ([]biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("graph node v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).GraphNodeV2.Query().
		Where(graphnodev2.GraphStageIDEQ(graphStageID)).
		Order(ent.Asc(graphnodev2.FieldID)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodesV2ToBiz(rows), nil
}

// CreateGraphNode inserts a new GraphNode.
// DependsOn is NOT persisted (in-memory only).
func (r *graphNodeV2Repo) CreateGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).GraphNodeV2.Create().
		SetID(gn.ID).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetTeamStageID(gn.TeamStageID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status)).
		Save(ctx)
	if err != nil {
		return biz.GraphNode{}, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodeV2ToBiz(row), nil
}

// UpdateGraphNode patches mutable fields.
// DependsOn is NOT persisted (in-memory only).
func (r *graphNodeV2Repo) UpdateGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).GraphNodeV2.UpdateOneID(gn.ID).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetTeamStageID(gn.TeamStageID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status)).
		Save(ctx)
	if err != nil {
		return biz.GraphNode{}, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodeV2ToBiz(row), nil
}

// UpsertGraphNode applies idempotent upsert for GraphNode.
// GraphNode has no Version field; uses ConstraintError fallback for idempotency.
// DependsOn is NOT persisted (in-memory only).
func (r *graphNodeV2Repo) UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	// Try update first.
	if err := r.data.RW().Write(ctx).GraphNodeV2.UpdateOneID(gn.ID).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetTeamStageID(gn.TeamStageID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status)).
		Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).GraphNodeV2.Get(ctx, gn.ID)
		if getErr != nil {
			return biz.GraphNode{}, entErrToBizErr(getErr, "GRAPH_NODE_V2")
		}
		return entGraphNodeV2ToBiz(row), nil
	}
	// Fallback to create.
	row, err := r.data.RW().Write(ctx).GraphNodeV2.Create().
		SetID(gn.ID).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetTeamStageID(gn.TeamStageID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status)).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).GraphNodeV2.Get(ctx, gn.ID)
			if getErr != nil {
				return biz.GraphNode{}, entErrToBizErr(getErr, "GRAPH_NODE_V2")
			}
			return entGraphNodeV2ToBiz(existing), nil
		}
		return biz.GraphNode{}, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodeV2ToBiz(row), nil
}

// entGraphNodeV2ToBiz converts an Ent GraphNodeV2 row to biz.GraphNode.
// DependsOn is intentionally left nil — it is an in-memory field derived from PlanStep.
func entGraphNodeV2ToBiz(row *ent.GraphNodeV2) biz.GraphNode {
	return biz.GraphNode{
		ID:           row.ID,
		GraphStageID: row.GraphStageID,
		Label:        row.Label,
		DagNodeID:    row.DagNodeID,
		TeamStageID:  row.TeamStageID,
		Status:       biz.GraphNodeStatus(row.Status),
		// DependsOn intentionally not set: in-memory only, derived from PlanStep
	}
}

func entGraphNodesV2ToBiz(rows []*ent.GraphNodeV2) []biz.GraphNode {
	out := make([]biz.GraphNode, 0, len(rows))
	for _, r := range rows {
		out = append(out, entGraphNodeV2ToBiz(r))
	}
	return out
}
