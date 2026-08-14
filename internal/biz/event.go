package biz

import (
	"context"
	"strings"
	"time"
)

// Event 是 v2 模型的统一事件接口。
// 所有事件源（Projector/Service/Team/PlanExecutor）都通过 Sequencer.Publish 发布。
//
// 实现说明：本接口要求方法 SpiritSessionID() 和 TaskID()，因此各事件 struct
// 的对应字段使用小写名（taskID / spiritSessionID）以避免 Go "field and method
// with the same name" 编译错误。同包构造可使用字面量；跨包构造需通过工厂函数。
// Stability:stable
type Event interface {
	EventKind() EventKind
	SpiritSessionID() string
	TaskID() string
	// EntityID returns the ID of the primary entity carried by this event.
	// Used for dead-letter deduplication in the Sequencer.
	EntityID() string
	// OccurredAt 返回事件发生时间（用于排序兜底，主排序用 Seq）
	OccurredAt() time.Time
}

// IsTerminalEventKind reports whether kind represents a terminal lifecycle
// state (completed/failed/cancelled/interrupted/skipped). B-06: these events
// must not be silently dropped on EventBus or WS queues.
func IsTerminalEventKind(kind EventKind) bool {
	k := string(kind)
	return strings.HasSuffix(k, ".completed") ||
		strings.HasSuffix(k, ".failed") ||
		strings.HasSuffix(k, ".cancelled") ||
		strings.HasSuffix(k, ".interrupted") ||
		strings.HasSuffix(k, ".skipped")
}

// IsCriticalDeliveryEvent reports whether e must use BlockUpTo / high-priority
// delivery (B-06). Covers suffix-terminal kinds plus payloads that encode
// terminal lifecycle without a *.completed/*.failed kind:
//   - plan_board.updated when PlanBoard status is terminal
//   - system.notice for orchestration_completed / orchestration_failed
func IsCriticalDeliveryEvent(e Event) bool {
	if e == nil {
		return false
	}
	if IsTerminalEventKind(e.EventKind()) {
		return true
	}
	switch ev := e.(type) {
	case *PlanBoardUpdatedEvent:
		return IsPlanBoardTerminal(ev.PlanBoard.Status)
	case *SystemNoticeEvent:
		switch ev.NoticeType {
		case "orchestration_completed", "orchestration_failed":
			return true
		}
	}
	return false
}

// EventKind 标识事件类型，格式为 "<entity>.<action>"。
type EventKind string

const (
	// Task 事件
	EventKindTaskCreated   EventKind = "task.created"
	EventKindTaskUpdated   EventKind = "task.updated"
	EventKindTaskCompleted EventKind = "task.completed"
	EventKindTaskFailed    EventKind = "task.failed"
	EventKindTaskCancelled EventKind = "task.cancelled"

	// Turn 事件
	EventKindTurnStarted   EventKind = "turn.started"
	EventKindTurnCompleted EventKind = "turn.completed"
	EventKindTurnFailed    EventKind = "turn.failed"

	// Step 事件
	EventKindStepCreated   EventKind = "step.created"
	EventKindStepStreaming EventKind = "step.streaming"
	EventKindStepUpdated   EventKind = "step.updated"
	EventKindStepCompleted EventKind = "step.completed"
	EventKindStepFailed    EventKind = "step.failed"

	// TeamStage 事件
	EventKindTeamStageCreated   EventKind = "team_stage.created"
	EventKindTeamStageUpdated   EventKind = "team_stage.updated"
	EventKindTeamStageCompleted EventKind = "team_stage.completed"
	EventKindTeamStageFailed    EventKind = "team_stage.failed"

	// TeamRun 事件
	EventKindTeamRunStarted   EventKind = "team_run.started"
	EventKindTeamRunCompleted EventKind = "team_run.completed"
	EventKindTeamRunFailed    EventKind = "team_run.failed"

	// MemberSession 事件
	EventKindMemberSessionCreated EventKind = "member_session.created"
	EventKindMemberSessionUpdated EventKind = "member_session.updated"

	// PlanBoard 事件
	EventKindPlanBoardCreated EventKind = "plan_board.created"
	EventKindPlanBoardUpdated EventKind = "plan_board.updated"

	// PlanStep 事件
	EventKindPlanStepStarted   EventKind = "plan_step.started"
	EventKindPlanStepCompleted EventKind = "plan_step.completed"
	EventKindPlanStepFailed    EventKind = "plan_step.failed"
	EventKindPlanStepSkipped   EventKind = "plan_step.skipped"
	EventKindPlanStepUpdated   EventKind = "plan_step.updated"

	// GraphStage 事件（2026-07-04 补齐：原设计遗漏，迁移 v1 ActivityKindGraphStage）
	EventKindGraphStageCreated     EventKind = "graph_stage.created"
	EventKindGraphStageUpdated     EventKind = "graph_stage.updated"
	EventKindGraphStageCompleted   EventKind = "graph_stage.completed"
	EventKindGraphStageFailed      EventKind = "graph_stage.failed"
	EventKindGraphStageInterrupted EventKind = "graph_stage.interrupted"
	EventKindGraphNodeUpdated      EventKind = "graph_node.updated"
)

// === Task 事件 ===

type TaskCreatedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCreatedEvent) EventKind() EventKind      { return EventKindTaskCreated }
func (e *TaskCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskCreatedEvent) TaskID() string            { return e.taskID }
func (e *TaskCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskUpdatedEvent) EventKind() EventKind      { return EventKindTaskUpdated }
func (e *TaskUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskUpdatedEvent) TaskID() string            { return e.taskID }
func (e *TaskUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskCompletedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCompletedEvent) EventKind() EventKind      { return EventKindTaskCompleted }
func (e *TaskCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskCompletedEvent) TaskID() string            { return e.taskID }
func (e *TaskCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskFailedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskFailedEvent) EventKind() EventKind      { return EventKindTaskFailed }
func (e *TaskFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskFailedEvent) TaskID() string            { return e.taskID }
func (e *TaskFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === Turn 事件 ===

type TurnStartedEvent struct {
	taskID          string
	spiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnStartedEvent) EventKind() EventKind      { return EventKindTurnStarted }
func (e *TurnStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnStartedEvent) TaskID() string            { return e.taskID }
func (e *TurnStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnCompletedEvent) EventKind() EventKind      { return EventKindTurnCompleted }
func (e *TurnCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnCompletedEvent) TaskID() string            { return e.taskID }
func (e *TurnCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnFailedEvent struct {
	taskID          string
	spiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnFailedEvent) EventKind() EventKind      { return EventKindTurnFailed }
func (e *TurnFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnFailedEvent) TaskID() string            { return e.taskID }
func (e *TurnFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === Step 事件 ===

type StepCreatedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCreatedEvent) EventKind() EventKind      { return EventKindStepCreated }
func (e *StepCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepCreatedEvent) TaskID() string            { return e.taskID }
func (e *StepCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// StepStreamingEvent 是流式增量事件（不入库，仅推送 WS）。
// Sequencer 16ms 批合并同 StepID + 同 DeltaField 的事件。
type StepStreamingEvent struct {
	taskID          string
	spiritSessionID string
	StepID          string
	DeltaField      string // content / reasoning / tool_args
	DeltaChunk      string
	// DeltaSeq 是 Sequencer 在 flush 时分配的会话级单调序号（与实体事件共享
	// SeqAssigner 计数空间）。前端按 (StepID, DeltaField) 记录最后应用的
	// DeltaSeq，丢弃 <= lastSeen 的重发/乱序增量 —— 取代内容指纹去重
	// （指纹会误杀合法的连续相同 chunk，如"哈哈哈"分三个"哈"到达）。
	DeltaSeq   int64
	occurredAt time.Time
}

func (e *StepStreamingEvent) EventKind() EventKind      { return EventKindStepStreaming }
func (e *StepStreamingEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepStreamingEvent) TaskID() string            { return e.taskID }
func (e *StepStreamingEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepStreamingEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepUpdatedEvent) EventKind() EventKind      { return EventKindStepUpdated }
func (e *StepUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepUpdatedEvent) TaskID() string            { return e.taskID }
func (e *StepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepCompletedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCompletedEvent) EventKind() EventKind      { return EventKindStepCompleted }
func (e *StepCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepCompletedEvent) TaskID() string            { return e.taskID }
func (e *StepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepFailedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepFailedEvent) EventKind() EventKind      { return EventKindStepFailed }
func (e *StepFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepFailedEvent) TaskID() string            { return e.taskID }
func (e *StepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamStage 事件 ===

type TeamStageCreatedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCreatedEvent) EventKind() EventKind      { return EventKindTeamStageCreated }
func (e *TeamStageCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageCreatedEvent) TaskID() string            { return e.taskID }
func (e *TeamStageCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageUpdatedEvent) EventKind() EventKind      { return EventKindTeamStageUpdated }
func (e *TeamStageUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageUpdatedEvent) TaskID() string            { return e.taskID }
func (e *TeamStageUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCompletedEvent) EventKind() EventKind      { return EventKindTeamStageCompleted }
func (e *TeamStageCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageCompletedEvent) TaskID() string            { return e.taskID }
func (e *TeamStageCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageFailedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageFailedEvent) EventKind() EventKind      { return EventKindTeamStageFailed }
func (e *TeamStageFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageFailedEvent) TaskID() string            { return e.taskID }
func (e *TeamStageFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamRun 事件 ===

type TeamRunStartedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunStartedEvent) EventKind() EventKind      { return EventKindTeamRunStarted }
func (e *TeamRunStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunStartedEvent) TaskID() string            { return e.taskID }
func (e *TeamRunStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunCompletedEvent) EventKind() EventKind      { return EventKindTeamRunCompleted }
func (e *TeamRunCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunCompletedEvent) TaskID() string            { return e.taskID }
func (e *TeamRunCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunFailedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunFailedEvent) EventKind() EventKind      { return EventKindTeamRunFailed }
func (e *TeamRunFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunFailedEvent) TaskID() string            { return e.taskID }
func (e *TeamRunFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === MemberSession 事件 ===

type MemberSessionCreatedEvent struct {
	taskID          string
	spiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionCreatedEvent) EventKind() EventKind      { return EventKindMemberSessionCreated }
func (e *MemberSessionCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *MemberSessionCreatedEvent) TaskID() string            { return e.taskID }
func (e *MemberSessionCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type MemberSessionUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionUpdatedEvent) EventKind() EventKind      { return EventKindMemberSessionUpdated }
func (e *MemberSessionUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *MemberSessionUpdatedEvent) TaskID() string            { return e.taskID }
func (e *MemberSessionUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanBoard 事件 ===

type PlanBoardCreatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardCreatedEvent) EventKind() EventKind      { return EventKindPlanBoardCreated }
func (e *PlanBoardCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanBoardCreatedEvent) TaskID() string            { return e.taskID }
func (e *PlanBoardCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanBoardUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardUpdatedEvent) EventKind() EventKind      { return EventKindPlanBoardUpdated }
func (e *PlanBoardUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanBoardUpdatedEvent) TaskID() string            { return e.taskID }
func (e *PlanBoardUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanStep 事件 ===

type PlanStepStartedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepStartedEvent) EventKind() EventKind      { return EventKindPlanStepStarted }
func (e *PlanStepStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepStartedEvent) TaskID() string            { return e.taskID }
func (e *PlanStepStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepCompletedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepCompletedEvent) EventKind() EventKind      { return EventKindPlanStepCompleted }
func (e *PlanStepCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepCompletedEvent) TaskID() string            { return e.taskID }
func (e *PlanStepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepFailedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepFailedEvent) EventKind() EventKind      { return EventKindPlanStepFailed }
func (e *PlanStepFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepFailedEvent) TaskID() string            { return e.taskID }
func (e *PlanStepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepSkippedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	Reason          string // dependency_failed / cancelled
	occurredAt      time.Time
}

func (e *PlanStepSkippedEvent) EventKind() EventKind      { return EventKindPlanStepSkipped }
func (e *PlanStepSkippedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepSkippedEvent) TaskID() string            { return e.taskID }
func (e *PlanStepSkippedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepSkippedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepUpdatedEvent) EventKind() EventKind      { return EventKindPlanStepUpdated }
func (e *PlanStepUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepUpdatedEvent) TaskID() string            { return e.taskID }
func (e *PlanStepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === GraphStage 事件（2026-07-04 补齐）===
// 替代 v1 ActivityKindGraphStage（通过 activity.bridge 桥接到 v2）。
// 与 PlanBoard 一对一关联：PlanBoard 创建时同步创建 GraphStage，状态由 PlanExecutor 同步。

type GraphStageCreatedEvent struct {
	taskID          string
	spiritSessionID string
	GraphStage      GraphStage
	occurredAt      time.Time
}

func (e *GraphStageCreatedEvent) EventKind() EventKind      { return EventKindGraphStageCreated }
func (e *GraphStageCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphStageCreatedEvent) TaskID() string            { return e.taskID }
func (e *GraphStageCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphStageCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type GraphStageUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	GraphStage      GraphStage
	occurredAt      time.Time
}

func (e *GraphStageUpdatedEvent) EventKind() EventKind      { return EventKindGraphStageUpdated }
func (e *GraphStageUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphStageUpdatedEvent) TaskID() string            { return e.taskID }
func (e *GraphStageUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphStageUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type GraphStageCompletedEvent struct {
	taskID          string
	spiritSessionID string
	GraphStage      GraphStage
	occurredAt      time.Time
}

func (e *GraphStageCompletedEvent) EventKind() EventKind      { return EventKindGraphStageCompleted }
func (e *GraphStageCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphStageCompletedEvent) TaskID() string            { return e.taskID }
func (e *GraphStageCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphStageCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type GraphStageFailedEvent struct {
	taskID          string
	spiritSessionID string
	GraphStage      GraphStage
	occurredAt      time.Time
}

func (e *GraphStageFailedEvent) EventKind() EventKind      { return EventKindGraphStageFailed }
func (e *GraphStageFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphStageFailedEvent) TaskID() string            { return e.taskID }
func (e *GraphStageFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphStageFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type GraphStageInterruptedEvent struct {
	taskID          string
	spiritSessionID string
	GraphStage      GraphStage
	occurredAt      time.Time
}

func (e *GraphStageInterruptedEvent) EventKind() EventKind      { return EventKindGraphStageInterrupted }
func (e *GraphStageInterruptedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphStageInterruptedEvent) TaskID() string            { return e.taskID }
func (e *GraphStageInterruptedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphStageInterruptedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// GraphNodeUpdatedEvent 携带单个 GraphNode 的状态更新。
// PlanExecutor 在 dispatchStep 时为对应 GraphNode 发布此事件。
type GraphNodeUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	GraphNode       GraphNode
	occurredAt      time.Time
}

func (e *GraphNodeUpdatedEvent) EventKind() EventKind      { return EventKindGraphNodeUpdated }
func (e *GraphNodeUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *GraphNodeUpdatedEvent) TaskID() string            { return e.taskID }
func (e *GraphNodeUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *GraphNodeUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === v2 EventBus ===

// EventBus is the v2 publish/subscribe bus for Events.
// Replaces the v1 ActivityEventBus. Implemented by event.V2Bus and adapters.
// Stability: evolving
type EventBus interface {
	Publish(ctx context.Context, e Event)
	Subscribe(opts EventSubscribeOptions) (<-chan Event, func())
}

// EventSubscribeOptions configures a subscription (e.g. filtering by session).
// Empty options = receive all events.
type EventSubscribeOptions struct {
	SpiritSessionID string
	TaskID          string
}
