package biz

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeterministicPlanStepID is the PublishV2Board id for a SubTask
// (plan + raw stage id). Used to rehydrate memory-only org fields after DB reload.
func DeterministicPlanStepID(planID, rawID string) string {
	planID = strings.TrimSpace(planID)
	rawID = strings.TrimSpace(rawID)
	if planID == "" || rawID == "" {
		return ""
	}
	return "st_" + uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.plan_step.v2:"+planID+":"+rawID)).String()
}

// HydratePlanStepsFromSubTasks copies collection_ids / confirm_before /
// graph_template_id from TaskPlan.SubTasks onto PlanSteps. Empty source
// fields do not overwrite values already set on the step.
func HydratePlanStepsFromSubTasks(planID string, steps []PlanStep, subtasks []SubTask) {
	if len(steps) == 0 || len(subtasks) == 0 {
		return
	}
	byStepID := make(map[string]SubTask, len(subtasks))
	for _, st := range subtasks {
		if id := DeterministicPlanStepID(planID, st.ID); id != "" {
			byStepID[id] = st
		}
		if id := strings.TrimSpace(st.ID); id != "" {
			if _, ok := byStepID[id]; !ok {
				byStepID[id] = st
			}
		}
	}
	for i := range steps {
		st, ok := byStepID[steps[i].ID]
		if !ok {
			continue
		}
		if steps[i].GraphTemplateID == "" {
			steps[i].GraphTemplateID = strings.TrimSpace(st.GraphTemplateID)
		}
		if !steps[i].ConfirmBefore {
			steps[i].ConfirmBefore = st.ConfirmBefore
		}
		if len(steps[i].CollectionIDs) == 0 {
			steps[i].CollectionIDs = NormalizeCollectionIDs(st.CollectionIDs)
		}
	}
}

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
	// Mode is the intra-team Graph mode (sequential/parallel/coordinator).
	// Filled at PublishV2Board / dispatch from planner strategy + member count.
	// Not a DB column: crash recovery infers from AgentKeys length.
	Mode string
	// P1 形式契约（B.10.15.2）：来自 SubTask，持久化到 plan_steps_v2，
	// dagRun 启动时做 advisory 契约验证；AssembleTeam 时透传到 Team。
	Deliverables  []DeliverableContract
	InputContract []DeliverableContract
	// DepartmentID is the team's home department (M78). Memory field — crash
	// recovery infers from member positions when empty (same pattern as Mode).
	DepartmentID string
	// CrossDeptMemberKeys are borrow candidates (agent keys). Memory field.
	CrossDeptMemberKeys []string
	// Staffing fields are memory-only (WS plan_step payload) so the frontend
	// can show specialty → person while the team is still assembling.
	DomainPath   string
	AssignedName string
	MatchLayer   string
	MatchReason  string
	// GraphTemplateID optionally routes this stage through an existing M53
	// template. Memory field copied from SubTask; empty = ordinary Team Turn.
	GraphTemplateID string
	// ConfirmBefore holds dispatch until the user approves (R18).
	ConfirmBefore bool
	// CollectionIDs scope knowledge_search for this stage. Memory field.
	CollectionIDs []string
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
