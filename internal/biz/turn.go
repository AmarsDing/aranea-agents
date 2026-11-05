package biz

import "time"

// Turn 是最小对话单元（v2 模型）：一次 LLM 回合。
// 一个 Task 包含多个 Turn；Team member 也有自己的 Turn（嵌套，三层封顶）。
type Turn struct {
	ID              string
	TaskID          string
	SessionID       string // 当前 session（spirit 或 team 或 member session）
	SpiritSessionID string // 始终指向 spirit root session（WS 过滤用）
	ParentTurnID    string // 嵌套时填：team member 的 turn 的 parent 是 team_stage 的某个 run turn
	AgentKey        string // 谁的 turn
	TeamID          string // 所属 team（spirit turn 为空）
	TeamStageID     string // 所属 team_stage（member turn 时填）
	Seq             int64  // 在 task 内的全局序号（后端分配，单调递增）
	Version         int64  // 乐观并发版本号（spec §3.3.5 VersionLT）
	Status          TurnStatus
	StartedAt       time.Time
	CompletedAt     *time.Time
	// RuntimeInvocationID 是框架侧本次 run 的真实 invocation id
	// （trpc 出库后框架 uuid ≠ 应用 turn id），由 v2 projector 自首条
	// 根事件捕获并随 turn 终态落库；空串 = 旧行或未捕获。仅供 session
	// fork 事件边界定位（migration 20261307），不参与 WS 语义。
	RuntimeInvocationID string
}

type TurnStatus string

const (
	TurnStatusRunning   TurnStatus = "running"
	TurnStatusCompleted TurnStatus = "completed"
	TurnStatusFailed    TurnStatus = "failed"
	TurnStatusCancelled TurnStatus = "cancelled"
)
