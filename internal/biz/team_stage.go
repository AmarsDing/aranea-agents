package biz

import "time"

// TeamStage 是 task 内的团队执行阶段（v2 模型）。
// 一个 Task 可包含多个 TeamStage（串行或并行），每个 TeamStage 对应一个 plan_step（如有）。
type TeamStage struct {
	ID          string
	TaskID      string
	TurnID      string // 触发 team 的 turn
	SessionID   string // spirit_session_id
	TeamID      string
	TeamName    string   // 团队显示名称（2026-07-04 问题 3 修复：UI 需要展示团队名称而非 ID）
	DagNodeID   string   // 对应 plan_step.id（如有）
	DependsOn   []string // 其他 team_stage.id（DAG 依赖）
	Status      TeamStageStatus
	Stage       TeamStageStage
	Members     []MemberInfo // 类型安全
	Strategy    string       // parallel/dag/coordinator
	StartedAt   time.Time
	CompletedAt *time.Time
	Seq         int64 // 在 task 内的序号
	Version     int64 // 乐观并发版本号（spec §3.3.5 VersionLT）
}

type TeamStageStatus string

const (
	TeamStageStatusPending      TeamStageStatus = "pending"
	TeamStageStatusRunning      TeamStageStatus = "running"
	TeamStageStatusCompleted    TeamStageStatus = "completed"
	TeamStageStatusFailed       TeamStageStatus = "failed"
	TeamStageStatusCancelled    TeamStageStatus = "cancelled"
	TeamStageStatusWaitingHuman TeamStageStatus = "waiting_human" // HITL
)

type TeamStageStage string

const (
	TeamStageStageAssembled TeamStageStage = "assembled"
	TeamStageStagePlanning  TeamStageStage = "planning"
	TeamStageStageExecuting TeamStageStage = "executing"
	TeamStageStageCompleted TeamStageStage = "completed"
	TeamStageStageFailed    TeamStageStage = "failed"
)

// MemberInfo 是 TeamStage 中的成员信息（类型安全，替代 Meta.members）。
type MemberInfo struct {
	AgentKey       string
	AgentName      string
	AvatarURL      string
	ChildSessionID string // member 自己的 session ID
	Status         string // pending/running/completed/failed/skipped
}
