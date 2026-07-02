package service

import (
	"context"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// planExecutorReposAdapter composes 3 v2 repo interfaces into the
// executorRepos interface required by PlanExecutor.
//
//   - biz.PlanStepV2Repo   provides UpsertPlanStep + GetPlanStep
//   - biz.TeamStageV2Repo  provides UpsertTeamStage
//   - biz.PlanBoardV2Repo  provides UpsertPlanBoard
//
// All underlying repos apply the VersionLT optimistic-concurrency guard
// (spec §3.3.5) inside their Upsert methods.
type planExecutorReposAdapter struct {
	planStep  biz.PlanStepV2Repo
	teamStage biz.TeamStageV2Repo
	planBoard biz.PlanBoardV2Repo
}

func newPlanExecutorReposAdapter(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
) executorRepos {
	return &planExecutorReposAdapter{
		planStep:  planStep,
		teamStage: teamStage,
		planBoard: planBoard,
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

func (a *planExecutorReposAdapter) GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error) {
	return a.planStep.GetPlanStep(ctx, id)
}

// NewPlanExecutorFromV2Repos is the Wire-friendly entry point for PlanExecutor.
// It accepts only exported types and internally builds the unexported
// executorRepos adapter (composing 3 v2 repo interfaces) before calling
// NewPlanExecutor. The v2.Sequencer satisfies the unexported sequencerPublisher
// interface (Publish(ctx, biz.Event)).
//
// Phase 1: orch should be the stub TeamOrchestrator (newStubTeamOrchestrator).
// Phase 2 will replace it with the real SpiritTeamAssembler-backed orchestrator.
func NewPlanExecutorFromV2Repos(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
	orch TeamOrchestrator,
	seq *v2.Sequencer,
	lg loggateway.Logger,
) *PlanExecutor {
	repos := newPlanExecutorReposAdapter(planStep, teamStage, planBoard)
	return NewPlanExecutor(repos, orch, seq, lg)
}
