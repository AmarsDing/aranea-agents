package biz

import "context"

// v2 Repo 窄接口（每个接口方法 ≤ 5，按读写职责拆分）。
// 复合接口仅用于 Wire 绑定。
//
// Stability:evolving
//
// 命名说明：使用 SpiritSession / SpiritSessionStatus / TeamRunV2Status 等
// Tier 0 命名（避免与 v1 的 Session 别名和 TeamRunStatus 字符串常量冲突）。

// === Session ===

type SessionV2Reader interface {
	GetSession(ctx context.Context, id string) (SpiritSession, error)
}

type SessionV2Writer interface {
	CreateSession(ctx context.Context, s SpiritSession) (SpiritSession, error)
	UpdateSessionStatus(ctx context.Context, id string, status SpiritSessionStatus) error
}

type SessionV2Repo interface {
	SessionV2Reader
	SessionV2Writer
}

// === Task ===

type TaskV2Reader interface {
	GetTask(ctx context.Context, id string) (Task, error)
	ListTasksBySession(ctx context.Context, sessionID string) ([]Task, error)
}

type TaskV2Writer interface {
	CreateTask(ctx context.Context, t Task) (Task, error)
	UpdateTask(ctx context.Context, t Task) (Task, error)
	UpsertTask(ctx context.Context, t Task) (Task, error)
}

type TaskV2Repo interface {
	TaskV2Reader
	TaskV2Writer
}

// === Turn ===

type TurnV2Reader interface {
	GetTurn(ctx context.Context, id string) (Turn, error)
	ListTurnsByTask(ctx context.Context, taskID string) ([]Turn, error)
}

type TurnV2Writer interface {
	CreateTurn(ctx context.Context, t Turn) (Turn, error)
	UpdateTurn(ctx context.Context, t Turn) (Turn, error)
	UpsertTurn(ctx context.Context, t Turn) (Turn, error)
}

type TurnV2Repo interface {
	TurnV2Reader
	TurnV2Writer
}

// === Step ===

type StepV2Reader interface {
	GetStep(ctx context.Context, id string) (Step, error)
	ListStepsByTurn(ctx context.Context, turnID string) ([]Step, error)
	ListStepsByTask(ctx context.Context, taskID string) ([]Step, error)
	ListStepsBySession(ctx context.Context, sessionID string) ([]Step, error) // replaces v1 ListBySession
	// MaxSeqBySpiritSession returns MAX(seq) for the spirit session, or 0 if none.
	// B-06: used to restore SeqAssigner after process restart.
	MaxSeqBySpiritSession(ctx context.Context, spiritSessionID string) (int64, error)
}

type StepV2Writer interface {
	CreateStep(ctx context.Context, s Step) (Step, error)
	UpdateStep(ctx context.Context, s Step) (Step, error)
	UpsertStep(ctx context.Context, s Step) (Step, error)
}

type StepV2Repo interface {
	StepV2Reader
	StepV2Writer
}

// === TeamStage ===

type TeamStageV2Reader interface {
	GetTeamStage(ctx context.Context, id string) (TeamStage, error)
	ListTeamStagesByTask(ctx context.Context, taskID string) ([]TeamStage, error)
}

type TeamStageV2Writer interface {
	CreateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpdateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpsertTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
}

type TeamStageV2Repo interface {
	TeamStageV2Reader
	TeamStageV2Writer
}

// === TeamRun ===

type TeamRunV2Reader interface {
	GetTeamRun(ctx context.Context, id string) (TeamRun, error)
	ListTeamRunsByStage(ctx context.Context, stageID string) ([]TeamRun, error)
}

type TeamRunV2Writer interface {
	CreateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpsertTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
}

type TeamRunV2Repo interface {
	TeamRunV2Reader
	TeamRunV2Writer
}

// === MemberSession ===

type MemberSessionV2Reader interface {
	GetMemberSession(ctx context.Context, id string) (MemberSession, error)
	ListMemberSessionsByRun(ctx context.Context, runID string) ([]MemberSession, error)
}

type MemberSessionV2Writer interface {
	CreateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpdateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpsertMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
}

type MemberSessionV2Repo interface {
	MemberSessionV2Reader
	MemberSessionV2Writer
}

// === PlanBoard ===

type PlanBoardV2Reader interface {
	GetPlanBoard(ctx context.Context, id string) (PlanBoard, error)
	ListPlanBoardsByTask(ctx context.Context, taskID string) ([]PlanBoard, error)
}

type PlanBoardV2Writer interface {
	CreatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpdatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpsertPlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
}

type PlanBoardV2Repo interface {
	PlanBoardV2Reader
	PlanBoardV2Writer
}

// === PlanStep ===

type PlanStepV2Reader interface {
	GetPlanStep(ctx context.Context, id string) (PlanStep, error)
	ListPlanStepsByPlan(ctx context.Context, planID string) ([]PlanStep, error)
	ListPlanStepsByTask(ctx context.Context, taskID string) ([]PlanStep, error)
}

type PlanStepV2Writer interface {
	CreatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpdatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpsertPlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
}

type PlanStepV2Repo interface {
	PlanStepV2Reader
	PlanStepV2Writer
}

// === GraphStage ===（2026-07-04 补齐：与 PlanBoard 一对一关联）

type GraphStageV2Reader interface {
	GetGraphStage(ctx context.Context, id string) (GraphStage, error)
	ListGraphStagesByTask(ctx context.Context, taskID string) ([]GraphStage, error)
	GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (GraphStage, error)
}

type GraphStageV2Writer interface {
	CreateGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
	UpdateGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
	UpsertGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
}

type GraphStageV2Repo interface {
	GraphStageV2Reader
	GraphStageV2Writer
}

// === GraphNode ===（GraphNode 独立存储，便于 team_stage 创建时回填 team_stage_id）

type GraphNodeV2Reader interface {
	GetGraphNode(ctx context.Context, id string) (GraphNode, error)
	ListGraphNodesByStage(ctx context.Context, graphStageID string) ([]GraphNode, error)
}

type GraphNodeV2Writer interface {
	CreateGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
	UpdateGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
	UpsertGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
}

type GraphNodeV2Repo interface {
	GraphNodeV2Reader
	GraphNodeV2Writer
}
