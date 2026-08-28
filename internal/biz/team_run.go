package biz

import "time"

// TeamRun 是 team 内的一次执行（v2 模型）。
// 一个 TeamStage 可包含多个 TeamRun（重试或并行批次）。
type TeamRun struct {
	ID              string
	TeamStageID     string
	TaskID          string
	SessionID       string
	SpiritSessionID string
	DagNodeID       string // 对应 plan_step.id
	DependsOn       []string
	Status          TeamRunV2Status
	StartedAt       time.Time
	CompletedAt     *time.Time
	Seq             int64
	Version         int64  // 乐观并发版本号（spec §3.3.5 VersionLT）
	Error           string // 失败时的错误信息（空字符串表示无错误）
}

// TeamRunV2Status 是 v2 TeamRun 的状态类型。
// 命名说明：本类型命名为 TeamRunV2Status 而非 TeamRunStatus，因为 biz 包
// 已在 team_run_state_machine.go 中定义了 TeamRunStatusRunning/Failed/Cancelled
// 等无类型字符串常量。使用 TeamRunV2Status 避免常量名冲突。
type TeamRunV2Status string

const (
	TeamRunV2StatusRunning   TeamRunV2Status = "running"
	TeamRunV2StatusPaused    TeamRunV2Status = "paused"
	TeamRunV2StatusCompleted TeamRunV2Status = "completed"
	TeamRunV2StatusFailed    TeamRunV2Status = "failed"
	TeamRunV2StatusCancelled TeamRunV2Status = "cancelled"
	// TeamRunV2StatusPartialFailure：run 完成但 ≥1 成员失败（F10）。终态。
	TeamRunV2StatusPartialFailure TeamRunV2Status = "partial_failure"
)
