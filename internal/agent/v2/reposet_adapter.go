package v2

import (
	"context"

	"aranea-agents/internal/biz"
)

// repoSetAdapter composes the 8 v2 repo interfaces into a single RepoSet
// implementation suitable for the Sequencer. All methods delegate to the
// underlying repo; each repo is responsible for its own optimistic-concurrency
// semantics (VersionLT guard — see spec §3.3.5).
type repoSetAdapter struct {
	task          biz.TaskV2Repo
	turn          biz.TurnV2Repo
	step          biz.StepV2Repo
	teamStage     biz.TeamStageV2Repo
	teamRun       biz.TeamRunV2Repo
	memberSession biz.MemberSessionV2Repo
	planBoard     biz.PlanBoardV2Repo
	planStep      biz.PlanStepV2Repo
}

// NewRepoSetAdapter composes 8 v2 repo interfaces into a RepoSet.
// All parameters must be non-nil; nil adapters will panic on first use of
// the corresponding Upsert method.
func NewRepoSetAdapter(
	task biz.TaskV2Repo,
	turn biz.TurnV2Repo,
	step biz.StepV2Repo,
	teamStage biz.TeamStageV2Repo,
	teamRun biz.TeamRunV2Repo,
	memberSession biz.MemberSessionV2Repo,
	planBoard biz.PlanBoardV2Repo,
	planStep biz.PlanStepV2Repo,
) RepoSet {
	return &repoSetAdapter{
		task:          task,
		turn:          turn,
		step:          step,
		teamStage:     teamStage,
		teamRun:       teamRun,
		memberSession: memberSession,
		planBoard:     planBoard,
		planStep:      planStep,
	}
}

func (a *repoSetAdapter) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	return a.task.UpsertTask(ctx, t)
}

func (a *repoSetAdapter) UpsertTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	return a.turn.UpsertTurn(ctx, t)
}

func (a *repoSetAdapter) UpsertStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	return a.step.UpsertStep(ctx, s)
}

func (a *repoSetAdapter) UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	return a.teamStage.UpsertTeamStage(ctx, ts)
}

func (a *repoSetAdapter) UpsertTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	return a.teamRun.UpsertTeamRun(ctx, tr)
}

func (a *repoSetAdapter) UpsertMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	return a.memberSession.UpsertMemberSession(ctx, ms)
}

func (a *repoSetAdapter) UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	return a.planBoard.UpsertPlanBoard(ctx, pb)
}

func (a *repoSetAdapter) UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	return a.planStep.UpsertPlanStep(ctx, ps)
}
