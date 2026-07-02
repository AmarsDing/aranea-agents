package biz

import "time"

// Event 是 v2 模型的统一事件接口。
// 所有事件源（Projector/Service/Team/PlanExecutor）都通过 Sequencer.Publish 发布。
//
// 实现说明：本接口要求方法 SpiritSessionID() 和 TaskID()，因此各事件 struct
// 的对应字段使用小写名（taskID / spiritSessionID）以避免 Go "field and method
// with the same name" 编译错误。同包构造可使用字面量；跨包构造需通过工厂函数。
type Event interface {
	EventKind() EventKind
	SpiritSessionID() string
	TaskID() string
	// OccurredAt 返回事件发生时间（用于排序兜底，主排序用 Seq）
	OccurredAt() time.Time
}

// EventKind 标识事件类型，格式为 "<entity>.<action>"。
type EventKind string

const (
	// Task 事件
	EventKindTaskCreated  EventKind = "task.created"
	EventKindTaskUpdated  EventKind = "task.updated"
	EventKindTaskCompleted EventKind = "task.completed"
	EventKindTaskFailed    EventKind = "task.failed"
	EventKindTaskCancelled EventKind = "task.cancelled"

	// Turn 事件
	EventKindTurnStarted   EventKind = "turn.started"
	EventKindTurnCompleted EventKind = "turn.completed"
	EventKindTurnFailed    EventKind = "turn.failed"

	// Step 事件
	EventKindStepCreated  EventKind = "step.created"
	EventKindStepStreaming EventKind = "step.streaming"
	EventKindStepUpdated  EventKind = "step.updated"
	EventKindStepCompleted EventKind = "step.completed"
	EventKindStepFailed    EventKind = "step.failed"

	// TeamStage 事件
	EventKindTeamStageCreated  EventKind = "team_stage.created"
	EventKindTeamStageUpdated  EventKind = "team_stage.updated"
	EventKindTeamStageCompleted EventKind = "team_stage.completed"
	EventKindTeamStageFailed   EventKind = "team_stage.failed"

	// TeamRun 事件
	EventKindTeamRunStarted  EventKind = "team_run.started"
	EventKindTeamRunCompleted EventKind = "team_run.completed"
	EventKindTeamRunFailed   EventKind = "team_run.failed"

	// MemberSession 事件
	EventKindMemberSessionCreated EventKind = "member_session.created"
	EventKindMemberSessionUpdated EventKind = "member_session.updated"

	// PlanBoard 事件
	EventKindPlanBoardCreated EventKind = "plan_board.created"
	EventKindPlanBoardUpdated EventKind = "plan_board.updated"

	// PlanStep 事件
	EventKindPlanStepStarted  EventKind = "plan_step.started"
	EventKindPlanStepCompleted EventKind = "plan_step.completed"
	EventKindPlanStepFailed   EventKind = "plan_step.failed"
	EventKindPlanStepSkipped  EventKind = "plan_step.skipped"
	EventKindPlanStepUpdated  EventKind = "plan_step.updated"
)

// === Task 事件 ===

type TaskCreatedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCreatedEvent) EventKind() EventKind       { return EventKindTaskCreated }
func (e *TaskCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskCreatedEvent) TaskID() string             { return e.taskID }
func (e *TaskCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskUpdatedEvent) EventKind() EventKind       { return EventKindTaskUpdated }
func (e *TaskUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskUpdatedEvent) TaskID() string             { return e.taskID }
func (e *TaskUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskCompletedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCompletedEvent) EventKind() EventKind       { return EventKindTaskCompleted }
func (e *TaskCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskCompletedEvent) TaskID() string             { return e.taskID }
func (e *TaskCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskFailedEvent struct {
	taskID          string
	spiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskFailedEvent) EventKind() EventKind       { return EventKindTaskFailed }
func (e *TaskFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TaskFailedEvent) TaskID() string             { return e.taskID }
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

func (e *TurnStartedEvent) EventKind() EventKind       { return EventKindTurnStarted }
func (e *TurnStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnStartedEvent) TaskID() string             { return e.taskID }
func (e *TurnStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnCompletedEvent) EventKind() EventKind       { return EventKindTurnCompleted }
func (e *TurnCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnCompletedEvent) TaskID() string             { return e.taskID }
func (e *TurnCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnFailedEvent struct {
	taskID          string
	spiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnFailedEvent) EventKind() EventKind       { return EventKindTurnFailed }
func (e *TurnFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TurnFailedEvent) TaskID() string             { return e.taskID }
func (e *TurnFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === Step 事件 ===

type StepCreatedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCreatedEvent) EventKind() EventKind       { return EventKindStepCreated }
func (e *StepCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepCreatedEvent) TaskID() string             { return e.taskID }
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
	occurredAt      time.Time
}

func (e *StepStreamingEvent) EventKind() EventKind       { return EventKindStepStreaming }
func (e *StepStreamingEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepStreamingEvent) TaskID() string             { return e.taskID }
func (e *StepStreamingEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepStreamingEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepUpdatedEvent) EventKind() EventKind       { return EventKindStepUpdated }
func (e *StepUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepUpdatedEvent) TaskID() string             { return e.taskID }
func (e *StepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepCompletedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCompletedEvent) EventKind() EventKind       { return EventKindStepCompleted }
func (e *StepCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepCompletedEvent) TaskID() string             { return e.taskID }
func (e *StepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepFailedEvent struct {
	taskID          string
	spiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepFailedEvent) EventKind() EventKind       { return EventKindStepFailed }
func (e *StepFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *StepFailedEvent) TaskID() string             { return e.taskID }
func (e *StepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamStage 事件 ===

type TeamStageCreatedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCreatedEvent) EventKind() EventKind       { return EventKindTeamStageCreated }
func (e *TeamStageCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageCreatedEvent) TaskID() string             { return e.taskID }
func (e *TeamStageCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageUpdatedEvent) EventKind() EventKind       { return EventKindTeamStageUpdated }
func (e *TeamStageUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageUpdatedEvent) TaskID() string             { return e.taskID }
func (e *TeamStageUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCompletedEvent) EventKind() EventKind       { return EventKindTeamStageCompleted }
func (e *TeamStageCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageCompletedEvent) TaskID() string             { return e.taskID }
func (e *TeamStageCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageFailedEvent struct {
	taskID          string
	spiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageFailedEvent) EventKind() EventKind       { return EventKindTeamStageFailed }
func (e *TeamStageFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamStageFailedEvent) TaskID() string             { return e.taskID }
func (e *TeamStageFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamRun 事件 ===

type TeamRunStartedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunStartedEvent) EventKind() EventKind       { return EventKindTeamRunStarted }
func (e *TeamRunStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunStartedEvent) TaskID() string             { return e.taskID }
func (e *TeamRunStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunCompletedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunCompletedEvent) EventKind() EventKind       { return EventKindTeamRunCompleted }
func (e *TeamRunCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunCompletedEvent) TaskID() string             { return e.taskID }
func (e *TeamRunCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunFailedEvent struct {
	taskID          string
	spiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunFailedEvent) EventKind() EventKind       { return EventKindTeamRunFailed }
func (e *TeamRunFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *TeamRunFailedEvent) TaskID() string             { return e.taskID }
func (e *TeamRunFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === MemberSession 事件 ===

type MemberSessionCreatedEvent struct {
	taskID          string
	spiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionCreatedEvent) EventKind() EventKind       { return EventKindMemberSessionCreated }
func (e *MemberSessionCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *MemberSessionCreatedEvent) TaskID() string             { return e.taskID }
func (e *MemberSessionCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type MemberSessionUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionUpdatedEvent) EventKind() EventKind       { return EventKindMemberSessionUpdated }
func (e *MemberSessionUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *MemberSessionUpdatedEvent) TaskID() string             { return e.taskID }
func (e *MemberSessionUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanBoard 事件 ===

type PlanBoardCreatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardCreatedEvent) EventKind() EventKind       { return EventKindPlanBoardCreated }
func (e *PlanBoardCreatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanBoardCreatedEvent) TaskID() string             { return e.taskID }
func (e *PlanBoardCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanBoardUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardUpdatedEvent) EventKind() EventKind       { return EventKindPlanBoardUpdated }
func (e *PlanBoardUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanBoardUpdatedEvent) TaskID() string             { return e.taskID }
func (e *PlanBoardUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanStep 事件 ===

type PlanStepStartedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepStartedEvent) EventKind() EventKind       { return EventKindPlanStepStarted }
func (e *PlanStepStartedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepStartedEvent) TaskID() string             { return e.taskID }
func (e *PlanStepStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepCompletedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepCompletedEvent) EventKind() EventKind       { return EventKindPlanStepCompleted }
func (e *PlanStepCompletedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepCompletedEvent) TaskID() string             { return e.taskID }
func (e *PlanStepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepFailedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepFailedEvent) EventKind() EventKind       { return EventKindPlanStepFailed }
func (e *PlanStepFailedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepFailedEvent) TaskID() string             { return e.taskID }
func (e *PlanStepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepSkippedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	Reason          string // dependency_failed / cancelled
	occurredAt      time.Time
}

func (e *PlanStepSkippedEvent) EventKind() EventKind       { return EventKindPlanStepSkipped }
func (e *PlanStepSkippedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepSkippedEvent) TaskID() string             { return e.taskID }
func (e *PlanStepSkippedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepSkippedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepUpdatedEvent struct {
	taskID          string
	spiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepUpdatedEvent) EventKind() EventKind       { return EventKindPlanStepUpdated }
func (e *PlanStepUpdatedEvent) SpiritSessionID() string   { return e.spiritSessionID }
func (e *PlanStepUpdatedEvent) TaskID() string             { return e.taskID }
func (e *PlanStepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }
