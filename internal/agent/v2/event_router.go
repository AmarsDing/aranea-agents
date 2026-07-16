// Package v2 implements the Phase 1 sequencer and projector for the
// LLM activity ordering redesign. See:
//
//	docs/superpowers/plans/2026-07-02-llm-activity-ordering-phase1.md
package v2

import (
	"context"

	"aranea-agents/internal/biz"
)

// RepoSet bundles all v2 repos needed by the Sequencer to persist events.
// Each method takes the entity extracted from an Event and upserts it.
// Implementations must be safe for concurrent use.
type RepoSet interface {
	UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error)
	UpsertTurn(ctx context.Context, t biz.Turn) (biz.Turn, error)
	UpsertStep(ctx context.Context, s biz.Step) (biz.Step, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error)
	UpsertMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	// 2026-07-04 问题 2 修复：补齐 GraphStage/GraphNode 持久化方法，
	// 让 GraphStageCreatedEvent / GraphNodeUpdatedEvent 能经过 sequencer 落库。
	UpsertGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error)
	UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error)
}

// persistAction routes an Event to the appropriate Repo method.
// Returns false if the event should NOT be persisted (e.g. step.streaming).
func persistAction(ctx context.Context, rs RepoSet, e biz.Event) (persisted bool, err error) {
	switch ev := e.(type) {
	case *biz.TaskCreatedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskUpdatedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskCompletedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskFailedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err

	case *biz.TurnStartedEvent:
		_, err = rs.UpsertTurn(ctx, ev.Turn)
		return true, err
	case *biz.TurnCompletedEvent:
		_, err = rs.UpsertTurn(ctx, ev.Turn)
		return true, err

	case *biz.StepCreatedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepUpdatedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepCompletedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepFailedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepStreamingEvent:
		// streaming chunks are NOT persisted; only pushed to WS
		return false, nil

	case *biz.TeamStageCreatedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageUpdatedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageCompletedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageFailedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err

	case *biz.TeamRunStartedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err
	case *biz.TeamRunCompletedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err
	case *biz.TeamRunFailedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err

	case *biz.MemberSessionCreatedEvent:
		_, err = rs.UpsertMemberSession(ctx, ev.MemberSession)
		return true, err
	case *biz.MemberSessionUpdatedEvent:
		_, err = rs.UpsertMemberSession(ctx, ev.MemberSession)
		return true, err

	case *biz.PlanBoardCreatedEvent:
		_, err = rs.UpsertPlanBoard(ctx, ev.PlanBoard)
		return true, err
	case *biz.PlanBoardUpdatedEvent:
		_, err = rs.UpsertPlanBoard(ctx, ev.PlanBoard)
		return true, err
	case *biz.PlanStepUpdatedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepStartedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepCompletedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepFailedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepSkippedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err

	// 2026-07-04 问题 2 修复：补齐 GraphStage/GraphNode 持久化 case。
	// 之前这些事件 fall through 到 default（return false, nil），不持久化，
	// 导致刷新后 graph_stage/graph_nodes 表为空，前端流程图消失。
	case *biz.GraphStageCreatedEvent:
		_, err = rs.UpsertGraphStage(ctx, ev.GraphStage)
		return true, err
	case *biz.GraphStageUpdatedEvent:
		_, err = rs.UpsertGraphStage(ctx, ev.GraphStage)
		return true, err
	case *biz.GraphStageCompletedEvent:
		_, err = rs.UpsertGraphStage(ctx, ev.GraphStage)
		return true, err
	case *biz.GraphStageFailedEvent:
		_, err = rs.UpsertGraphStage(ctx, ev.GraphStage)
		return true, err
	case *biz.GraphStageInterruptedEvent:
		_, err = rs.UpsertGraphStage(ctx, ev.GraphStage)
		return true, err
	case *biz.GraphNodeUpdatedEvent:
		_, err = rs.UpsertGraphNode(ctx, ev.GraphNode)
		return true, err
	}
	// Unknown event type: skip persistence
	return false, nil
}
