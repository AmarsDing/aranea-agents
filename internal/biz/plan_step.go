package biz

import "time"

// PlanStep 是计划步骤（v2 模型）。
// 替代旧 Plan/PlanStep（废弃）+ SubTask 双轨模型。
type PlanStep struct {
	ID                string
	PlanID            string
	TaskID            string // 冗余，便于按 task 索引
	Label             string
	Description       string
	DependsOn         []string // 其他 plan_step.id
	MappedTeamStageID string   // 执行该 step 的 team_stage（如有；coordinator 模式下所有 step 共享一个 team_stage）
	Status            PlanStepStatus
	AutoSynthesis     bool // 是否为汇总报告 step（无 team 映射，依赖完成自动触发）
	StartedAt         time.Time
	CompletedAt       *time.Time
	Seq               int64       // 在 plan 内的序号
	Version           int64       // 乐观并发版本号（spec §3.3.5 VersionLT）
	Result            *StepResult // 完成时携带
	Error             *StepError  // 失败时携带
	// AgentKeys 是 LLM 分配给该 step 的 agent key 列表（来自 AllocationPlan）。
	// RealTeamOrchestrator 优先使用此字段组建 team，避免查 DB 取到错误 agent。
	// 2026-07-05 Step 2 修复：解决"所有 team 用同一 agent"问题。
	AgentKeys []string
	// P1 形式契约（B.10.15.2）：来自 SubTask，持久化到 plan_steps_v2，
	// dagRun 启动时做 advisory 契约验证；AssembleTeam 时透传到 Team。
	Deliverables  []DeliverableContract
	InputContract []DeliverableContract
}

type PlanStepStatus string

const (
	PlanStepStatusPending        PlanStepStatus = "pending"
	PlanStepStatusRunning        PlanStepStatus = "running"
	PlanStepStatusCompleted      PlanStepStatus = "completed"
	PlanStepStatusFailed         PlanStepStatus = "failed"
	PlanStepStatusSkipped        PlanStepStatus = "skipped" // 依赖失败导致跳过
	PlanStepStatusPartialFailure PlanStepStatus = "partial_failure"
)

// StepResult 是 plan step 完成时携带的结果。
type StepResult struct {
	Output        string
	MemberReports []MemberReport
	TokensUsed    TokenUsage
	DurationMs    int64
}

// StepError 是 plan step 失败时携带的错误。
type StepError struct {
	Code         string // tool_failed / llm_timeout / team_failed / dependency_failed
	Message      string
	Retryable    bool
	FailedMember *MemberReport // 哪个 member 失败
}

// MemberReport 是单个 member 的执行报告。
type MemberReport struct {
	AgentKey   string
	AgentName  string
	Output     string
	TokensUsed TokenUsage
	DurationMs int64
	Error      string // 失败时填
}

// TokenUsage 是 token 用量统计。
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}
