// Package v2 implements the Phase 1 sequencer and projector for the
// LLM activity ordering redesign. See:
//
//	docs/superpowers/plans/2026-07-02-llm-activity-ordering-phase1.md
package v2

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
)

// RepoSet bundles all v2 repos needed by the Sequencer to persist events.
// Each method takes the entity extracted from an Event and upserts it.
// Implementations must be safe for concurrent use.
type RepoSet interface {
	UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error)
	// CompleteTaskTerminal persists task.completed / task.failed (L3,
	// 2026-07-22). Terminal events are monotonic: version is bumped from the
	// DB value, so a stale event Version (e.g. the synthesis turn's hardcoded
	// Version=2 after a resume CAS pushed it higher) cannot strand the task
	// in running. Already-terminal tasks are not overwritten.
	CompleteTaskTerminal(ctx context.Context, t biz.Task) (biz.Task, error)
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

// Entity kinds handled by the persist router. Stored in the dead-letter
// table so replay can route without knowing the original event kind.
const (
	EntityKindTask          = "task"
	EntityKindTurn          = "turn"
	EntityKindStep          = "step"
	EntityKindTeamStage     = "team_stage"
	EntityKindTeamRun       = "team_run"
	EntityKindMemberSession = "member_session"
	EntityKindPlanBoard     = "plan_board"
	EntityKindPlanStep      = "plan_step"
	EntityKindGraphStage    = "graph_stage"
	EntityKindGraphNode     = "graph_node"
)

// Persist operations.
const (
	PersistOpUpsert               = "upsert"
	PersistOpCompleteTaskTerminal = "complete_task_terminal"
)

// persistDescriptor maps an event to its entity-level persist action. It is
// the single source of truth for event→entity routing: live persist
// (persistAction), dead-letter save (entity marshal) and dead-letter replay
// (entity decode) all derive from it.
type persistDescriptor struct {
	entityKind string
	op         string
	entity     any // entity VALUE (biz.Task, biz.Turn, ...)
}

// describePersist returns the persist descriptor for an event, or nil when
// the event must NOT be persisted (streaming chunks, ephemeral system
// events, unknown types).
func describePersist(e biz.Event) *persistDescriptor {
	switch ev := e.(type) {
	case *biz.TaskCreatedEvent:
		return &persistDescriptor{EntityKindTask, PersistOpUpsert, ev.Task}
	case *biz.TaskUpdatedEvent:
		return &persistDescriptor{EntityKindTask, PersistOpUpsert, ev.Task}
	case *biz.TaskCompletedEvent:
		// L3: 终态事件走 CompleteTaskTerminal（version 从 DB +1），避免 resume
		// 后 synthesis OnTurnEnd 硬编码 Version=2 被 VersionLT guard 拒绝导致
		// task 永远 running。
		return &persistDescriptor{EntityKindTask, PersistOpCompleteTaskTerminal, ev.Task}
	case *biz.TaskFailedEvent:
		return &persistDescriptor{EntityKindTask, PersistOpCompleteTaskTerminal, ev.Task}

	case *biz.TurnStartedEvent:
		return &persistDescriptor{EntityKindTurn, PersistOpUpsert, ev.Turn}
	case *biz.TurnCompletedEvent:
		return &persistDescriptor{EntityKindTurn, PersistOpUpsert, ev.Turn}

	case *biz.StepCreatedEvent:
		return &persistDescriptor{EntityKindStep, PersistOpUpsert, ev.Step}
	case *biz.StepUpdatedEvent:
		return &persistDescriptor{EntityKindStep, PersistOpUpsert, ev.Step}
	case *biz.StepCompletedEvent:
		return &persistDescriptor{EntityKindStep, PersistOpUpsert, ev.Step}
	case *biz.StepFailedEvent:
		return &persistDescriptor{EntityKindStep, PersistOpUpsert, ev.Step}

	case *biz.TeamStageCreatedEvent:
		return &persistDescriptor{EntityKindTeamStage, PersistOpUpsert, ev.TeamStage}
	case *biz.TeamStageUpdatedEvent:
		return &persistDescriptor{EntityKindTeamStage, PersistOpUpsert, ev.TeamStage}
	case *biz.TeamStageCompletedEvent:
		return &persistDescriptor{EntityKindTeamStage, PersistOpUpsert, ev.TeamStage}
	case *biz.TeamStageFailedEvent:
		return &persistDescriptor{EntityKindTeamStage, PersistOpUpsert, ev.TeamStage}

	case *biz.TeamRunStartedEvent:
		return &persistDescriptor{EntityKindTeamRun, PersistOpUpsert, ev.TeamRun}
	case *biz.TeamRunCompletedEvent:
		return &persistDescriptor{EntityKindTeamRun, PersistOpUpsert, ev.TeamRun}
	case *biz.TeamRunFailedEvent:
		return &persistDescriptor{EntityKindTeamRun, PersistOpUpsert, ev.TeamRun}

	case *biz.MemberSessionCreatedEvent:
		return &persistDescriptor{EntityKindMemberSession, PersistOpUpsert, ev.MemberSession}
	case *biz.MemberSessionUpdatedEvent:
		return &persistDescriptor{EntityKindMemberSession, PersistOpUpsert, ev.MemberSession}

	case *biz.PlanBoardCreatedEvent:
		return &persistDescriptor{EntityKindPlanBoard, PersistOpUpsert, ev.PlanBoard}
	case *biz.PlanBoardUpdatedEvent:
		return &persistDescriptor{EntityKindPlanBoard, PersistOpUpsert, ev.PlanBoard}
	case *biz.PlanStepStartedEvent:
		return &persistDescriptor{EntityKindPlanStep, PersistOpUpsert, ev.PlanStep}
	case *biz.PlanStepCompletedEvent:
		return &persistDescriptor{EntityKindPlanStep, PersistOpUpsert, ev.PlanStep}
	case *biz.PlanStepFailedEvent:
		return &persistDescriptor{EntityKindPlanStep, PersistOpUpsert, ev.PlanStep}
	case *biz.PlanStepSkippedEvent:
		return &persistDescriptor{EntityKindPlanStep, PersistOpUpsert, ev.PlanStep}
	case *biz.PlanStepUpdatedEvent:
		return &persistDescriptor{EntityKindPlanStep, PersistOpUpsert, ev.PlanStep}

	case *biz.GraphStageCreatedEvent:
		return &persistDescriptor{EntityKindGraphStage, PersistOpUpsert, ev.GraphStage}
	case *biz.GraphStageUpdatedEvent:
		return &persistDescriptor{EntityKindGraphStage, PersistOpUpsert, ev.GraphStage}
	case *biz.GraphStageCompletedEvent:
		return &persistDescriptor{EntityKindGraphStage, PersistOpUpsert, ev.GraphStage}
	case *biz.GraphStageFailedEvent:
		return &persistDescriptor{EntityKindGraphStage, PersistOpUpsert, ev.GraphStage}
	case *biz.GraphStageInterruptedEvent:
		return &persistDescriptor{EntityKindGraphStage, PersistOpUpsert, ev.GraphStage}
	case *biz.GraphNodeUpdatedEvent:
		return &persistDescriptor{EntityKindGraphNode, PersistOpUpsert, ev.GraphNode}
	}
	// step.streaming / system.* / unknown: not persisted.
	return nil
}

// applyPersist executes the entity-level persist for a descriptor/replay
// record. Routes purely on entityKind + op, so it serves both the live path
// and dead-letter replay.
func applyPersist(ctx context.Context, rs RepoSet, entityKind, op string, entity any) error {
	switch entityKind {
	case EntityKindTask:
		t, ok := entity.(biz.Task)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.Task", entity)
		}
		if op == PersistOpCompleteTaskTerminal {
			_, err := rs.CompleteTaskTerminal(ctx, t)
			return err
		}
		_, err := rs.UpsertTask(ctx, t)
		return err
	case EntityKindTurn:
		v, ok := entity.(biz.Turn)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.Turn", entity)
		}
		_, err := rs.UpsertTurn(ctx, v)
		return err
	case EntityKindStep:
		v, ok := entity.(biz.Step)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.Step", entity)
		}
		_, err := rs.UpsertStep(ctx, v)
		return err
	case EntityKindTeamStage:
		v, ok := entity.(biz.TeamStage)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.TeamStage", entity)
		}
		_, err := rs.UpsertTeamStage(ctx, v)
		return err
	case EntityKindTeamRun:
		v, ok := entity.(biz.TeamRun)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.TeamRun", entity)
		}
		_, err := rs.UpsertTeamRun(ctx, v)
		return err
	case EntityKindMemberSession:
		v, ok := entity.(biz.MemberSession)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.MemberSession", entity)
		}
		_, err := rs.UpsertMemberSession(ctx, v)
		return err
	case EntityKindPlanBoard:
		v, ok := entity.(biz.PlanBoard)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.PlanBoard", entity)
		}
		_, err := rs.UpsertPlanBoard(ctx, v)
		return err
	case EntityKindPlanStep:
		v, ok := entity.(biz.PlanStep)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.PlanStep", entity)
		}
		_, err := rs.UpsertPlanStep(ctx, v)
		return err
	case EntityKindGraphStage:
		v, ok := entity.(biz.GraphStage)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.GraphStage", entity)
		}
		_, err := rs.UpsertGraphStage(ctx, v)
		return err
	case EntityKindGraphNode:
		v, ok := entity.(biz.GraphNode)
		if !ok {
			return fmt.Errorf("applyPersist: entity is %T, want biz.GraphNode", entity)
		}
		_, err := rs.UpsertGraphNode(ctx, v)
		return err
	}
	return fmt.Errorf("applyPersist: unknown entity kind %q", entityKind)
}

// decodePersistEntity rehydrates a dead-letter payload back into an entity
// value routable by applyPersist.
func decodePersistEntity(entityKind string, payload []byte) (any, error) {
	unmarshal := func(ptr any) error {
		if err := json.Unmarshal(payload, ptr); err != nil {
			return fmt.Errorf("decodePersistEntity(%s): %w", entityKind, err)
		}
		return nil
	}
	switch entityKind {
	case EntityKindTask:
		var v biz.Task
		return v, unmarshal(&v)
	case EntityKindTurn:
		var v biz.Turn
		return v, unmarshal(&v)
	case EntityKindStep:
		var v biz.Step
		return v, unmarshal(&v)
	case EntityKindTeamStage:
		var v biz.TeamStage
		return v, unmarshal(&v)
	case EntityKindTeamRun:
		var v biz.TeamRun
		return v, unmarshal(&v)
	case EntityKindMemberSession:
		var v biz.MemberSession
		return v, unmarshal(&v)
	case EntityKindPlanBoard:
		var v biz.PlanBoard
		return v, unmarshal(&v)
	case EntityKindPlanStep:
		var v biz.PlanStep
		return v, unmarshal(&v)
	case EntityKindGraphStage:
		var v biz.GraphStage
		return v, unmarshal(&v)
	case EntityKindGraphNode:
		var v biz.GraphNode
		return v, unmarshal(&v)
	}
	return nil, fmt.Errorf("decodePersistEntity: unknown entity kind %q", entityKind)
}

// persistAction routes an Event to the appropriate Repo method.
// Returns false if the event should NOT be persisted (e.g. step.streaming).
func persistAction(ctx context.Context, rs RepoSet, e biz.Event) (persisted bool, err error) {
	d := describePersist(e)
	if d == nil {
		return false, nil
	}
	return true, applyPersist(ctx, rs, d.entityKind, d.op, d.entity)
}
