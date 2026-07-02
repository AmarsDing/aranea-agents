package biz

import "time"

// PlanBoard 是任务计划面板（v2 模型）。
// 替代旧 TaskPlan + Activity.Meta.steps 双轨模型。
type PlanBoard struct {
	ID          string
	TaskID      string
	TurnID      string // 触发 plan 的 turn
	SessionID   string // spirit_session_id
	Strategy    PlanStrategy
	Status      PlanStatus
	Steps       []PlanStep // 数组，每个 step 有 id 和 depends_on
	StartedAt   time.Time
	CompletedAt *time.Time
	Seq         int64
	Version     int64 // 乐观并发版本号（spec §3.3.5 VersionLT）
}

type PlanStrategy string

const (
	PlanStrategySequential  PlanStrategy = "sequential"
	PlanStrategyParallel    PlanStrategy = "parallel"
	PlanStrategyDAG         PlanStrategy = "dag"
	PlanStrategyCoordinator PlanStrategy = "coordinator"
)

type PlanStatus string

const (
	PlanStatusPlanning       PlanStatus = "planning"
	PlanStatusExecuting      PlanStatus = "executing"
	PlanStatusCompleted      PlanStatus = "completed"
	PlanStatusFailed         PlanStatus = "failed"
	PlanStatusPartialFailure PlanStatus = "partial_failure"
)
