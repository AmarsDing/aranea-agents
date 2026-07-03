package biz

import "time"

// GraphStage 是 Graph 流程图（v2 模型），与 PlanBoard 一对一关联。
// 替代 v1 ActivityKindGraphStage（通过 activity.bridge 桥接到 v2）。
//
// 设计：见 docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.2 GraphStage / §3.7.5 GraphStageBlock 与 PlanDAG 的关系
//
// 关键关系：
//   - GraphStage.PlanBoardID 唯一指向一个 PlanBoard（一对一）
//   - GraphNode 由 PlanStep 派生：节点状态由 PlanStep.Status 实时映射
//   - GraphStage 是独立 entity（独立表 + 独立 Repo + 独立事件），不存储在 PlanBoard 内
//   - 持久化策略：与 PlanBoard 同步创建，状态由 PlanExecutor 在 dispatchStep/checkDownstream 时同步更新
type GraphStage struct {
	ID          string
	TaskID      string
	TurnID      string // 触发 graph 的 turn（plan 创建后）
	SessionID   string // spirit_session_id
	PlanBoardID string // 关联的 PlanBoard（一对一）
	Nodes       []GraphNode
	Status      GraphStageStatus
	StartedAt   time.Time
	CompletedAt *time.Time
	Seq         int64
	Version     int64 // 乐观并发版本号（spec §3.3.5 VersionLT）
}

type GraphStageStatus string

const (
	GraphStageStatusRunning     GraphStageStatus = "running"
	GraphStageStatusCompleted   GraphStageStatus = "completed"
	GraphStageStatusFailed      GraphStageStatus = "failed"
	GraphStageStatusInterrupted GraphStageStatus = "interrupted" // 暂停/中断
)

// GraphNode 是 GraphStage 内的一个节点，对应一个 PlanStep。
// 节点状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
type GraphNode struct {
	ID           string // 通常 = plan_step.id（确定性派生）
	GraphStageID string
	Label        string   // 取自 PlanStep.Label
	DagNodeID    string   // 对应 plan_step.id
	TeamStageID  string   // 关联的 team_stage（如已创建，否则空）
	Status       GraphNodeStatus
	DependsOn    []string // 取自 PlanStep.DependsOn（派生，不持久化）
}

type GraphNodeStatus string

const (
	GraphNodeStatusPending     GraphNodeStatus = "pending"     // 灰色
	GraphNodeStatusRunning     GraphNodeStatus = "running"     // 青色脉冲
	GraphNodeStatusCompleted   GraphNodeStatus = "completed"   // 绿色 ✓
	GraphNodeStatusFailed      GraphNodeStatus = "failed"       // 红色 ✗
	GraphNodeStatusInterrupted GraphNodeStatus = "interrupted"  // 黄色 ⏸（需求 §A.4.2）
)

// MapPlanStepToGraphNodeStatus 将 PlanStep.Status 映射为 GraphNodeStatus。
// 由 PlanExecutor 在 dispatchStep/checkDownstream 时同步调用。
//
// 映射规则（设计文档 §3.7.5）：
//   - pending → pending
//   - running → running
//   - completed → completed
//   - failed / partial_failure → failed
//   - skipped → interrupted（skipped 在 graph 上显示为 interrupted，黄色 ⏸）
func MapPlanStepToGraphNodeStatus(ps PlanStepStatus) GraphNodeStatus {
	switch ps {
	case PlanStepStatusPending:
		return GraphNodeStatusPending
	case PlanStepStatusRunning:
		return GraphNodeStatusRunning
	case PlanStepStatusCompleted:
		return GraphNodeStatusCompleted
	case PlanStepStatusFailed, PlanStepStatusPartialFailure:
		return GraphNodeStatusFailed
	case PlanStepStatusSkipped:
		return GraphNodeStatusInterrupted
	default:
		return GraphNodeStatusPending
	}
}
