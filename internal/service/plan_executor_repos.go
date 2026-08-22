package service

import (
	"context"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// planExecutorReposAdapter composes 5 v2 repo interfaces into the
// executorRepos interface required by PlanExecutor.
//
//   - biz.PlanStepV2Repo     provides UpsertPlanStep + GetPlanStep
//   - biz.TeamStageV2Repo    provides UpsertTeamStage
//   - biz.PlanBoardV2Repo    provides UpsertPlanBoard
//   - biz.GraphStageV2Repo   provides UpsertGraphStage + GetGraphStageByPlanBoard
//   - biz.GraphNodeV2Repo    provides UpsertGraphNode
//
// All underlying repos apply the VersionLT optimistic-concurrency guard
// (spec §3.3.5) inside their Upsert methods.
type planExecutorReposAdapter struct {
	planStep   biz.PlanStepV2Repo
	teamStage  biz.TeamStageV2Repo
	planBoard  biz.PlanBoardV2Repo
	graphStage biz.GraphStageV2Repo
	graphNode  biz.GraphNodeV2Repo
}

func newPlanExecutorReposAdapter(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
	graphStage biz.GraphStageV2Repo,
	graphNode biz.GraphNodeV2Repo,
) executorRepos {
	return &planExecutorReposAdapter{
		planStep:   planStep,
		teamStage:  teamStage,
		planBoard:  planBoard,
		graphStage: graphStage,
		graphNode:  graphNode,
	}
}

func (a *planExecutorReposAdapter) UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	return a.planStep.UpsertPlanStep(ctx, ps)
}

func (a *planExecutorReposAdapter) UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	return a.teamStage.UpsertTeamStage(ctx, ts)
}

func (a *planExecutorReposAdapter) UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	return a.planBoard.UpsertPlanBoard(ctx, pb)
}

func (a *planExecutorReposAdapter) GetPlanBoard(ctx context.Context, id string) (biz.PlanBoard, error) {
	return a.planBoard.GetPlanBoard(ctx, id)
}

func (a *planExecutorReposAdapter) GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error) {
	return a.planStep.GetPlanStep(ctx, id)
}

func (a *planExecutorReposAdapter) ListPlanStepsByPlan(ctx context.Context, planID string) ([]biz.PlanStep, error) {
	return a.planStep.ListPlanStepsByPlan(ctx, planID)
}

func (a *planExecutorReposAdapter) ListPlanBoardsByStatuses(ctx context.Context, statuses []biz.PlanStatus) ([]biz.PlanBoard, error) {
	return a.planBoard.ListPlanBoardsByStatuses(ctx, statuses)
}

// UpsertGraphStage delegates to GraphStageV2Repo (2026-07-04 补齐).
func (a *planExecutorReposAdapter) UpsertGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error) {
	return a.graphStage.UpsertGraphStage(ctx, gs)
}

// UpsertGraphNode delegates to GraphNodeV2Repo (2026-07-04 补齐).
func (a *planExecutorReposAdapter) UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	return a.graphNode.UpsertGraphNode(ctx, gn)
}

// GetGraphStageByPlanBoard delegates to GraphStageV2Repo (2026-07-04 补齐).
func (a *planExecutorReposAdapter) GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (biz.GraphStage, error) {
	return a.graphStage.GetGraphStageByPlanBoard(ctx, planBoardID)
}

// GetTeamStage delegates to TeamStageV2Repo (2026-07-05 P1 #9d 补齐).
// 用于读取当前 TeamStage 的 Version 和 Status，修复 Version 硬编码 Bug。
func (a *planExecutorReposAdapter) GetTeamStage(ctx context.Context, id string) (biz.TeamStage, error) {
	return a.teamStage.GetTeamStage(ctx, id)
}

// NewPlanExecutorFromV2Repos is the Wire-friendly entry point for PlanExecutor.
// It accepts only exported types and internally builds the unexported
// executorRepos adapter (composing 5 v2 repo interfaces) before calling
// NewPlanExecutor. The v2.Sequencer satisfies the unexported sequencerPublisher
// interface (Publish(ctx, biz.Event)).
//
// Phase 1: orch should be the stub TeamOrchestrator (newStubTeamOrchestrator).
// Phase 2 will replace it with the real SpiritTeamAssembler-backed orchestrator.
//
// 2026-07-04 补齐：新增 graphStage + graphNode 依赖，用于同步创建 GraphStage
// 和更新 GraphNode 状态（与 PlanBoard 一对一关联）。
func NewPlanExecutorFromV2Repos(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
	graphStage biz.GraphStageV2Repo,
	graphNode biz.GraphNodeV2Repo,
	orch TeamOrchestrator,
	seq *v2.Sequencer,
	lg loggateway.Logger,
) *PlanExecutor {
	repos := newPlanExecutorReposAdapter(planStep, teamStage, planBoard, graphStage, graphNode)
	return NewPlanExecutor(repos, orch, seq, lg)
}
