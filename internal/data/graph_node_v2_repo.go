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
// TeamStageID：仅非空时写入，避免空串擦除已回填关联。
func (r *graphNodeV2Repo) UpdateGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	upd := r.data.RW().Write(ctx).GraphNodeV2.UpdateOneID(gn.ID).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status))
	if gn.TeamStageID != "" {
		upd = upd.SetTeamStageID(gn.TeamStageID)
	}
	row, err := upd.Save(ctx)
	if err != nil {
		return biz.GraphNode{}, entErrToBizErr(err, "GRAPH_NODE_V2")
	}
	return entGraphNodeV2ToBiz(row), nil
}

// UpsertGraphNode applies idempotent upsert for GraphNode.
// GraphNode has no Version field; uses ConstraintError fallback for idempotency.
// DependsOn is NOT persisted (in-memory only).
//
// TeamStageID：Update 时仅在非空时写入，避免终态 status 更新用空串擦除
// dispatch 已回填的 team_stage 关联（chat GraphStageBlock 点击跳转依赖此字段）。
//
// FR-11（89 批次3）：非终态 incoming status 的 update 追加终态谓词
// （StatusNotIn completed/failed/interrupted）——sequencer 死信重放携带的陈旧
// running 快照不得把已终态节点覆写回中间态。0 行命中即发生了守卫拦截或行不
// 存在：先读分辨，行在 → 幂等返回现有行（镜像兄弟 repo 幂等语义）；行不在 →
// 走 Create。终态 incoming（completed/failed/interrupted）不设防——终态单调，
// 合法覆写（含死信重放的宕后收账）。
func (r *graphNodeV2Repo) UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	if r == nil || r.data == nil {
		return biz.GraphNode{}, fmt.Errorf("graph node v2 repo: database not configured")
	}
	// Try update first. UpdateOneID 不可用——守卫谓词须挂在 bulk Update 上。
	upd := r.data.RW().Write(ctx).GraphNodeV2.Update().
		Where(graphnodev2.ID(gn.ID)).
		SetGraphStageID(gn.GraphStageID).
		SetDagNodeID(gn.DagNodeID).
		SetLabel(gn.Label).
		SetStatus(string(gn.Status))
	if gn.TeamStageID != "" {
		upd = upd.SetTeamStageID(gn.TeamStageID)
	}
	if !isTerminalGraphNodeStatus(gn.Status) {
		upd = upd.Where(graphnodev2.StatusNotIn(
			string(biz.GraphNodeStatusCompleted),
			string(biz.GraphNodeStatusFailed),
			string(biz.GraphNodeStatusInterrupted),
		))
	}
	n, err := upd.Save(ctx)
	if err == nil && n > 0 {
		row, getErr := r.data.RW().Read(ctx).GraphNodeV2.Get(ctx, gn.ID)
		if getErr != nil {
			return biz.GraphNode{}, entErrToBizErr(getErr, "GRAPH_NODE_V2")
		}
		return entGraphNodeV2ToBiz(row), nil
	}
	if err == nil {
		// n==0：守卫拦截（行已为终态）或行不存在——读一次分辨。
		existing, getErr := r.data.RW().Read(ctx).GraphNodeV2.Get(ctx, gn.ID)
		if getErr == nil {
			return entGraphNodeV2ToBiz(existing), nil
		}
		if !ent.IsNotFound(getErr) {
			return biz.GraphNode{}, entErrToBizErr(getErr, "GRAPH_NODE_V2")
		}
		// 行确不存在 → 落 Create。
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
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
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

// isTerminalGraphNodeStatus 报告 status 是否为 GraphNode 终态（FR-11 守卫
// 名单——与 graph 状态机终态集一致：completed/failed/interrupted；
// pending/running 为可逆中间态）。
func isTerminalGraphNodeStatus(s biz.GraphNodeStatus) bool {
	switch s {
	case biz.GraphNodeStatusCompleted, biz.GraphNodeStatusFailed, biz.GraphNodeStatusInterrupted:
		return true
	}
	return false
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
