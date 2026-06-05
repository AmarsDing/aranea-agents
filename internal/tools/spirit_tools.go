package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	biztypes "aranea-agents/internal/biz/types"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ---------------------------------------------------------------------------
// New three-phase orchestration tools (T3.1)
// ---------------------------------------------------------------------------

// PlanAndExecuteInput is the input for the plan_and_execute tool.
type PlanAndExecuteInput struct {
	TaskPrompt string `json:"task_prompt" jsonschema:"description=The task to plan and execute"`
	Mode       string `json:"mode,omitempty" jsonschema:"description=Execution mode: auto|direct|single|parallel|dag|coordinator (default: auto)"`
}

// SubTaskSummary is a summary of a subtask in the plan.
type SubTaskSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	AgentKey  string   `json:"agent_key,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// PlanAndExecuteOutput is the output for the plan_and_execute tool.
type PlanAndExecuteOutput struct {
	PlanID          string                          `json:"plan_id"`
	Strategy        string                          `json:"strategy"`
	ComplexityLevel string                          `json:"complexity_level"`
	SubtaskCount    int                             `json:"subtask_count"`
	SubTasks        []SubTaskSummary                `json:"sub_tasks,omitempty"`
	OrchestrationID string                          `json:"orchestration_id,omitempty"`
	MemoryHit       bool                            `json:"memory_hit"`
	Steps           []biztypes.OrchestrationStepRecord `json:"steps,omitempty"`
}

// planAndExecuteDeps holds the shared dependencies for plan_and_execute phases.
type planAndExecuteDeps struct {
	planner      biz.TaskPlannerPort
	allocator    biz.AgentAllocatorPort
	orchestrator biz.TaskOrchestratorPort
	bus          contract.Bus
	lg           loggateway.Logger
}

// NewPlanAndExecuteTool creates the plan_and_execute tool that replaces
// assess_complexity + assemble_team + list_butlers + query_butler_status.
func NewPlanAndExecuteTool(planner biz.TaskPlannerPort, allocator biz.AgentAllocatorPort, orchestrator biz.TaskOrchestratorPort, bus contract.Bus, lg loggateway.Logger) *trpcfunction.FunctionTool[PlanAndExecuteInput, PlanAndExecuteOutput] {
	deps := planAndExecuteDeps{
		planner:      planner,
		allocator:    allocator,
		orchestrator: orchestrator,
		bus:          bus,
		lg:           lg,
	}

	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input PlanAndExecuteInput) (PlanAndExecuteOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return PlanAndExecuteOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}

			taskPrompt := strings.TrimSpace(input.TaskPrompt)
			if taskPrompt == "" {
				return PlanAndExecuteOutput{}, kerrors.BadRequest("SPIRIT", "task_prompt is required")
			}

			// Emit ButlerOrchestrationStarted event.
			if deps.bus != nil {
				env := contract.NewEnvelope(contract.EnvelopeTypeButlerOrchestrationStarted, "plan_and_execute", spiritSessionID)
				env.Metadata = map[string]any{
					"task_prompt": taskPrompt,
					"mode":        input.Mode,
				}
				deps.bus.Publish(ctx, env)
			}

			var steps []biztypes.OrchestrationStepRecord

			// Phase 1: Plan
			taskPlan, planStep, err := executePlanPhase(ctx, taskPrompt, spiritSessionID, deps)
			steps = append(steps, planStep)
			if err != nil {
				publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "plan", err.Error())
				return PlanAndExecuteOutput{Steps: steps}, err
			}

			out := PlanAndExecuteOutput{
				PlanID:          taskPlan.ID,
				Strategy:        string(taskPlan.Strategy),
				ComplexityLevel: string(taskPlan.ComplexityLevel),
				SubtaskCount:    len(taskPlan.SubTasks),
				MemoryHit:       taskPlan.MemoryHit != nil,
				Steps:           steps,
			}
			for _, st := range taskPlan.SubTasks {
				out.SubTasks = append(out.SubTasks, SubTaskSummary{
					ID:        st.ID,
					Name:      st.Name,
					DependsOn: st.DependsOn,
				})
			}

			// For direct strategy, no allocation or orchestration needed.
			if taskPlan.Strategy == biz.StrategyDirect {
				publishOrchestrationCompleted(deps.bus, ctx, spiritSessionID, out.OrchestrationID, out.Strategy, len(out.SubTasks))
				return out, nil
			}

			// Phase 2: Allocate
			allocPlan, allocStep, allocErr := executeAllocatePhase(ctx, taskPlan, deps)
			out.Steps = append(out.Steps, allocStep)
			if allocErr != nil {
				publishOrchestrationCompleted(deps.bus, ctx, spiritSessionID, out.OrchestrationID, out.Strategy, len(out.SubTasks))
				return out, nil
			}

			// Fill agent keys from allocation into subtask summaries.
			for i := range out.SubTasks {
				for _, alloc := range allocPlan.Allocations {
					if alloc.SubTaskID == out.SubTasks[i].ID {
						out.SubTasks[i].AgentKey = alloc.AssignedKey
						break
					}
				}
			}

			// Phase 3: Orchestrate
			handle, orchStep, orchErr := executeOrchestratePhase(ctx, taskPlan, allocPlan, deps)
			out.Steps = append(out.Steps, orchStep)
			if orchErr != nil {
				publishOrchestrationCompleted(deps.bus, ctx, spiritSessionID, out.OrchestrationID, out.Strategy, len(out.SubTasks))
				return out, nil
			}

			out.OrchestrationID = handle.ID
			publishOrchestrationCompleted(deps.bus, ctx, spiritSessionID, out.OrchestrationID, out.Strategy, len(out.SubTasks))
			return out, nil
		},
		trpcfunction.WithName("plan_and_execute"),
		trpcfunction.WithDescription("规划并执行任务。自动评估复杂度、分配 Agent、启动编排。替代 assess_complexity + assemble_team 的组合调用。简单任务直接回答，复杂任务自动组建团队。"),
	)
}

// executePlanPhase runs Phase 1 of plan_and_execute: task planning.
func executePlanPhase(ctx context.Context, taskPrompt, spiritSessionID string, deps planAndExecuteDeps) (*biz.TaskPlan, biztypes.OrchestrationStepRecord, error) {
	start := time.Now().UTC()
	planInput := biz.PlanInput{
		UserMessage:     taskPrompt,
		SpiritSessionID: spiritSessionID,
	}
	taskPlan, err := deps.planner.Plan(ctx, planInput)
	if err != nil {
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "plan",
			Status:     "failed",
			Error:      err.Error(),
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, kerrors.InternalServer("SPIRIT", "plan failed: "+err.Error())
	}
	return taskPlan, biztypes.OrchestrationStepRecord{
		StepName:   "plan",
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
	}, nil
}

// executeAllocatePhase runs Phase 2 of plan_and_execute: agent allocation.
func executeAllocatePhase(ctx context.Context, taskPlan *biz.TaskPlan, deps planAndExecuteDeps) (*biz.AllocationPlan, biztypes.OrchestrationStepRecord, error) {
	start := time.Now().UTC()
	allocPlan, err := deps.allocator.Allocate(ctx, taskPlan)
	if err != nil {
		deps.lg.Warn("plan_and_execute: allocation failed, returning plan only",
			loggateway.StepID("spirit.plan_and_execute.alloc_fail"),
			loggateway.Str("plan_id", taskPlan.ID),
			loggateway.Err(err),
		)
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "allocate",
			Status:     "failed",
			Error:      err.Error(),
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, err
	}
	return allocPlan, biztypes.OrchestrationStepRecord{
		StepName:   "allocate",
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
	}, nil
}

// executeOrchestratePhase runs Phase 3 of plan_and_execute: task orchestration.
func executeOrchestratePhase(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, deps planAndExecuteDeps) (*biz.OrchestrationHandle, biztypes.OrchestrationStepRecord, error) {
	start := time.Now().UTC()
	handle, err := deps.orchestrator.Orchestrate(ctx, taskPlan, allocPlan)
	if err != nil {
		deps.lg.Warn("plan_and_execute: orchestration failed, returning plan only",
			loggateway.StepID("spirit.plan_and_execute.orch_fail"),
			loggateway.Str("plan_id", taskPlan.ID),
			loggateway.Err(err),
		)
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "orchestrate",
			Status:     "failed",
			Error:      err.Error(),
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, err
	}
	return handle, biztypes.OrchestrationStepRecord{
		StepName:   "orchestrate",
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
	}, nil
}

// publishOrchestrationFailed emits a ButlerOrchestrationFailed event.
func publishOrchestrationFailed(bus contract.Bus, ctx context.Context, sessionID, phase, errMsg string) {
	if bus == nil {
		return
	}
	env := contract.NewEnvelope(contract.EnvelopeTypeButlerOrchestrationFailed, "plan_and_execute", sessionID)
	env.Metadata = map[string]any{
		"phase": phase,
		"error": errMsg,
	}
	bus.Publish(ctx, env)
}

// publishOrchestrationCompleted emits a ButlerOrchestrationCompleted event.
func publishOrchestrationCompleted(bus contract.Bus, ctx context.Context, sessionID, orchestrationID, strategy string, subtaskCount int) {
	if bus == nil {
		return
	}
	env := contract.NewEnvelope(contract.EnvelopeTypeButlerOrchestrationCompleted, "plan_and_execute", sessionID)
	env.Metadata = map[string]any{
		"orchestration_id": orchestrationID,
		"strategy":         strategy,
		"subtask_count":    subtaskCount,
	}
	bus.Publish(ctx, env)
}

// CheckOrchestrationProgressInput is the input for the check_progress tool.
type CheckOrchestrationProgressInput struct {
	OrchestrationID string `json:"orchestration_id" jsonschema:"description=The orchestration ID to check progress for"`
}

// TaskProgressView is a view of a single task's progress.
type TaskProgressView struct {
	SubTaskID   string  `json:"sub_task_id"`
	SubTaskName string  `json:"sub_task_name"`
	AgentKey    string  `json:"agent_key"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
}

// CheckOrchestrationProgressOutput is the output for the check_progress tool.
type CheckOrchestrationProgressOutput struct {
	OrchestrationID string             `json:"orchestration_id"`
	Status          string             `json:"status"`
	Tasks           []TaskProgressView `json:"tasks"`
}

// NewCheckOrchestrationProgressTool creates the check_progress tool that replaces check_team_progress.
func NewCheckOrchestrationProgressTool(orchestrator biz.TaskOrchestratorPort, lg loggateway.Logger) *trpcfunction.FunctionTool[CheckOrchestrationProgressInput, CheckOrchestrationProgressOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CheckOrchestrationProgressInput) (CheckOrchestrationProgressOutput, error) {
			orchestrationID := strings.TrimSpace(input.OrchestrationID)
			if orchestrationID == "" {
				return CheckOrchestrationProgressOutput{}, kerrors.BadRequest("SPIRIT", "orchestration_id is required")
			}

			progress, err := orchestrator.CheckProgress(ctx, orchestrationID)
			if err != nil {
				return CheckOrchestrationProgressOutput{}, kerrors.InternalServer("SPIRIT", "check progress: "+err.Error())
			}

			views := make([]TaskProgressView, 0, len(progress))
			for _, p := range progress {
				views = append(views, TaskProgressView{
					SubTaskID:   p.SubTaskID,
					SubTaskName: p.SubTaskName,
					AgentKey:    p.AgentKey,
					Status:      p.Status,
					Progress:    p.Progress,
				})
			}
			return CheckOrchestrationProgressOutput{
				OrchestrationID: orchestrationID,
				Status:          "running",
				Tasks:           views,
			}, nil
		},
		trpcfunction.WithName("check_progress"),
		trpcfunction.WithDescription("查询编排执行进度。替代 check_team_progress，基于 orchestration_id 查询。"),
	)
}

// CancelOrchestrationInput is the input for the cancel_orchestration tool.
type CancelOrchestrationInput struct {
	OrchestrationID string `json:"orchestration_id" jsonschema:"description=The orchestration ID to cancel"`
}

// CancelOrchestrationOutput is the output for the cancel_orchestration tool.
type CancelOrchestrationOutput struct {
	OrchestrationID string `json:"orchestration_id"`
	Status          string `json:"status"`
}

// NewCancelOrchestrationTool creates the cancel_orchestration tool that replaces cancel_team.
func NewCancelOrchestrationTool(orchestrator biz.TaskOrchestratorPort, lg loggateway.Logger) *trpcfunction.FunctionTool[CancelOrchestrationInput, CancelOrchestrationOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CancelOrchestrationInput) (CancelOrchestrationOutput, error) {
			orchestrationID := strings.TrimSpace(input.OrchestrationID)
			if orchestrationID == "" {
				return CancelOrchestrationOutput{}, kerrors.BadRequest("SPIRIT", "orchestration_id is required")
			}
			err := orchestrator.Cancel(ctx, orchestrationID)
			if err != nil {
				return CancelOrchestrationOutput{}, err
			}
			return CancelOrchestrationOutput{OrchestrationID: orchestrationID, Status: "cancelled"}, nil
		},
		trpcfunction.WithName("cancel_orchestration"),
		trpcfunction.WithDescription("取消正在运行的编排。替代 cancel_team，基于 orchestration_id 取消。取消后释放资源。"),
	)
}

// ---------------------------------------------------------------------------
// TODO(debt): Remove deprecated tools after Spirit migration period (target: v0.4).
// These tools are no longer registered but kept for reference and backward compatibility.
// Ref: cross-module-coordination-unify
// ---------------------------------------------------------------------------
// Deprecated old tools — kept for backward compatibility and test coverage.
// These tools are NO LONGER REGISTERED in the Spirit Agent's tool list.
// Use plan_and_execute / check_progress / cancel_orchestration instead.
// ---------------------------------------------------------------------------

type AssembleTeamInput struct {
	AgentKeys   []string `json:"agent_keys"  jsonschema:"description=参与团队的 Agent key 列表"`
	Mode        string   `json:"mode"         jsonschema:"description=团队编排模式,enum=coordinator,enum=sequential,enum=parallel"`
	TaskPrompt  string   `json:"task_prompt"  jsonschema:"description=任务描述，用于生成团队名称和成员指令"`
	TaskDAGJSON string   `json:"task_dag_json,omitempty" jsonschema:"description=任务 DAG 的 JSON 描述（可选），包含节点和依赖关系。格式为 [{id,task_name,description,depends_on,mode,agent_keys}]"`
	AutoStart   *bool    `json:"auto_start,omitempty"  jsonschema:"description=是否自动启动团队执行（默认 true）"`
}

type AssembleTeamOutput struct {
	TeamID         string `json:"team_id"`
	SessionID      string `json:"session_id"`
	TeamName       string `json:"team_name"`
	TopologyReason string `json:"topology_reason,omitempty"`
	DAGDiagram     string `json:"dag_diagram,omitempty"`
}

type TeamProgressView struct {
	TeamID      string  `json:"team_id"`
	TeamName    string  `json:"team_name"`
	Status      string  `json:"status"`
	ProgressPct float64 `json:"progress_pct"`
	CurrentStep string  `json:"current_step"`
	DurationMs  int64   `json:"duration_ms"`
}

type SpiritTeamAssemblerPort interface {
	AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error)
	SuggestTopology(ctx context.Context, taskDescription string) (string, bool)
}

type SpiritTeamQueryPort interface {
	ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
}

type SpiritTeamControllerPort interface {
	CancelTeam(ctx context.Context, teamID string) error
	CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
}

type AssessComplexityInput struct {
	UserMessage string `json:"user_message" jsonschema:"description=用户消息内容"`
}

type AssessComplexityOutput struct {
	Level          string   `json:"level"`
	Reasoning      string   `json:"reasoning"`
	SuggestedPath  string   `json:"suggested_path"`
	RequiredSkills []string `json:"required_skills,omitempty"`
	AvailableTools []string `json:"available_tools"`
}

type SpiritSynthesisPort interface {
	SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error)
}

// NewAssembleTeamTool creates the deprecated assemble_team tool.
// DEPRECATED: Use plan_and_execute instead. This tool delegates to the new three-phase flow.
func NewAssembleTeamTool(assembler SpiritTeamAssemblerPort, query SpiritTeamQueryPort, lg loggateway.Logger, planner biz.TaskPlannerPort, allocator biz.AgentAllocatorPort, orchestrator biz.TaskOrchestratorPort) *trpcfunction.FunctionTool[AssembleTeamInput, AssembleTeamOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input AssembleTeamInput) (AssembleTeamOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return AssembleTeamOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}

			// Delegate to new three-phase flow when ports are available.
			if planner != nil && allocator != nil && orchestrator != nil {
				planInput := biz.PlanInput{
					UserMessage:     strings.TrimSpace(input.TaskPrompt),
					SpiritSessionID: spiritSessionID,
				}
				taskPlan, err := planner.Plan(ctx, planInput)
				if err != nil {
					return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "plan failed: "+err.Error())
				}
				allocPlan, err := allocator.Allocate(ctx, taskPlan)
				if err != nil {
					return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "allocate failed: "+err.Error())
				}
				handle, err := orchestrator.Orchestrate(ctx, taskPlan, allocPlan)
				if err != nil {
					return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "orchestrate failed: "+err.Error())
				}
				return AssembleTeamOutput{
					TeamID:    handle.ID,
					TeamName:  string(handle.Strategy),
					SessionID: spiritSessionID,
				}, nil
			}

			// Fallback to legacy flow when new ports are not available.
			activeTeams, err := query.ListActiveTeams(ctx, spiritSessionID)
			if err != nil {
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "query active teams: "+err.Error())
			}
			maxParallel := query.GetMaxParallelTeams(ctx, spiritSessionID)
			availableSlots := maxParallel - len(activeTeams)
			if availableSlots <= 0 {
				return AssembleTeamOutput{}, kerrors.BadRequest(
					"SPIRIT",
					fmt.Sprintf("max parallel teams (%d) reached, wait for existing teams to complete", maxParallel),
				)
			}

			mode := strings.TrimSpace(input.Mode)
			var topologyReason string

			dag, dagErr := biz.ParseTaskDAG(strings.TrimSpace(input.TaskDAGJSON), lg)
			if dagErr != nil {
				return AssembleTeamOutput{}, kerrors.BadRequest("SPIRIT", "invalid task dag: "+dagErr.Error())
			}

			if dag != nil && len(dag.Nodes) > 0 {
				routed := dag.RouteTopology()
				if mode == "" {
					mode = string(routed)
				}
				topologyReason = biz.FormatTopologyReason(routed, false, dag)
			}

			if cached, found := assembler.SuggestTopology(ctx, strings.TrimSpace(input.TaskPrompt)); found && cached != "" {
				if mode == "" {
					mode = cached
				}
				if topologyReason == "" {
					topologyReason = fmt.Sprintf("基于历史编排缓存推荐拓扑: %s", cached)
				}
			}
			if mode == "" {
				mode = "coordinator"
			}
			autoStart := true
			if input.AutoStart != nil {
				autoStart = *input.AutoStart
			}
			params := biz.SpiritTeamParams{
				SpiritSessionID:    spiritSessionID,
				TaskDescription:    strings.TrimSpace(input.TaskPrompt),
				AgentKeys:          input.AgentKeys,
				Mode:               mode,
				ParallelConfigJSON: buildParallelConfigJSON(maxParallel),
				TopologyReason:     topologyReason,
				AutoStart:          autoStart,
			}

			if dag != nil && len(dag.Nodes) > 1 {
				outputs, err := assembleDAGTeams(ctx, assembler, dag, spiritSessionID, mode, input.AgentKeys, maxParallel, availableSlots, autoStart)
				if err != nil {
					return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "assemble dag teams: "+err.Error())
				}
				if len(outputs) > 0 {
					outputs[0].DAGDiagram = dag.ToTextDiagram()
					outputs[0].TopologyReason = topologyReason
					return outputs[0], nil
				}
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "no teams created from dag")
			}

			if dag != nil && len(dag.Nodes) == 1 {
				for _, node := range dag.OrderedNodes() {
					params.DagNodeID = string(node.ID)
					dependsOn := make([]string, len(node.DependsOn))
					for i, d := range node.DependsOn {
						dependsOn[i] = string(d)
					}
					params.DependsOn = dependsOn
					break
				}
			}

			team, session, err := assembler.AssembleTeam(ctx, params)
			if err != nil {
				return AssembleTeamOutput{}, kerrors.InternalServer("SPIRIT", "assemble team: "+err.Error())
			}
			return AssembleTeamOutput{
				TeamID:         team.ID,
				SessionID:      session.ID,
				TeamName:       team.DisplayName,
				TopologyReason: topologyReason,
			}, nil
		},
		trpcfunction.WithName("assemble_team"),
		trpcfunction.WithDescription("[DEPRECATED] 组建任务团队。请改用 plan_and_execute 工具。当用户需求复杂、需要多 Agent 协作时调用此工具。"),
	)
}

type CheckTeamProgressInput struct{}

type CheckTeamProgressOutput struct {
	Teams []TeamProgressView `json:"teams"`
}

// NewCheckTeamProgressTool creates the deprecated check_team_progress tool.
// DEPRECATED: Use check_progress instead. This tool delegates to TaskOrchestratorPort.CheckProgress.
func NewCheckTeamProgressTool(controller SpiritTeamControllerPort, orchestrator biz.TaskOrchestratorPort) *trpcfunction.FunctionTool[CheckTeamProgressInput, CheckTeamProgressOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ CheckTeamProgressInput) (CheckTeamProgressOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return CheckTeamProgressOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}

			// Delegate to new orchestrator when available.
			if orchestrator != nil {
				progress, err := orchestrator.CheckProgress(ctx, spiritSessionID)
				if err != nil {
					return CheckTeamProgressOutput{}, kerrors.InternalServer("SPIRIT", "check progress: "+err.Error())
				}
				views := make([]TeamProgressView, 0, len(progress))
				for _, p := range progress {
					views = append(views, TeamProgressView{
						TeamName:    p.SubTaskName,
						Status:      p.Status,
						ProgressPct: p.Progress * 100,
					})
				}
				return CheckTeamProgressOutput{Teams: views}, nil
			}

			// Fallback to legacy flow.
			progress, err := controller.CheckTeamProgress(ctx, spiritSessionID)
			if err != nil {
				return CheckTeamProgressOutput{}, kerrors.InternalServer("SPIRIT", "check team progress: "+err.Error())
			}
			views := make([]TeamProgressView, 0, len(progress))
			for _, p := range progress {
				views = append(views, TeamProgressView{
					TeamID:      p.TeamID,
					TeamName:    p.TeamName,
					Status:      p.Status,
					ProgressPct: p.ProgressPct,
					CurrentStep: p.CurrentStep,
					DurationMs:  p.DurationMs,
				})
			}
			return CheckTeamProgressOutput{Teams: views}, nil
		},
		trpcfunction.WithName("check_team_progress"),
		trpcfunction.WithDescription("[DEPRECATED] 查询当前精灵会话下所有团队的执行进度。请改用 check_progress 工具。"),
	)
}

type CancelTeamInput struct {
	TeamID string `json:"team_id" jsonschema:"description=要取消的团队 ID"`
}

type CancelTeamOutput struct {
	TeamID string `json:"team_id"`
	Status string `json:"status"`
}

// NewCancelTeamTool creates the deprecated cancel_team tool.
// DEPRECATED: Use cancel_orchestration instead. This tool delegates to TaskOrchestratorPort.Cancel.
func NewCancelTeamTool(controller SpiritTeamControllerPort, orchestrator biz.TaskOrchestratorPort) *trpcfunction.FunctionTool[CancelTeamInput, CancelTeamOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CancelTeamInput) (CancelTeamOutput, error) {
			teamID := strings.TrimSpace(input.TeamID)
			if teamID == "" {
				return CancelTeamOutput{}, kerrors.BadRequest("SPIRIT", "team_id is required")
			}

			// Delegate to new orchestrator when available.
			if orchestrator != nil {
				err := orchestrator.Cancel(ctx, teamID)
				if err != nil {
					return CancelTeamOutput{}, err
				}
				return CancelTeamOutput{TeamID: teamID, Status: "cancelled"}, nil
			}

			// Fallback to legacy flow.
			err := controller.CancelTeam(ctx, teamID)
			if err != nil {
				return CancelTeamOutput{}, err
			}
			return CancelTeamOutput{TeamID: teamID, Status: "cancelled"}, nil
		},
		trpcfunction.WithName("cancel_team"),
		trpcfunction.WithDescription("[DEPRECATED] 取消正在运行的团队。请改用 cancel_orchestration 工具。"),
	)
}

type SynthesizeResultsInput struct {
	Strategy string `json:"strategy,omitempty" jsonschema:"description=合成策略,enum=template,enum=prompt,enum=hybrid"`
}

type SynthesizeResultsOutput struct {
	Content            string                    `json:"content"`
	Strategy           string                    `json:"strategy"`
	TeamResults        []biz.TeamSynthesisResult `json:"team_results"`
	NeedsLLMSynthesis  bool                      `json:"needs_llm_synthesis"`
}

func NewSynthesizeResultsTool(synthesis SpiritSynthesisPort) *trpcfunction.FunctionTool[SynthesizeResultsInput, SynthesizeResultsOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input SynthesizeResultsInput) (SynthesizeResultsOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return SynthesizeResultsOutput{}, kerrors.BadRequest("SPIRIT", "spirit session id not found in context")
			}
			output, err := synthesis.SynthesizeResults(ctx, spiritSessionID, input.Strategy)
			if err != nil {
				return SynthesizeResultsOutput{}, kerrors.InternalServer("SPIRIT", "synthesize results: "+err.Error())
			}
			return SynthesizeResultsOutput{
				Content:           output.Content,
				Strategy:          string(output.Strategy),
				TeamResults:       output.TeamResults,
				NeedsLLMSynthesis: output.Strategy == biz.SynthesisStrategyPrompt || output.Strategy == biz.SynthesisStrategyHybrid,
			}, nil
		},
		trpcfunction.WithName("synthesize_results"),
		trpcfunction.WithDescription("合成所有已完成团队的执行结果。当所有并行团队完成后调用此工具，将各团队结果整合为综合报告。"),
	)
}

func spiritSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}

func buildParallelConfigJSON(maxConcurrent int) string {
	if maxConcurrent <= 0 {
		return ""
	}
	cfg := biz.ParallelConfig{
		MaxConcurrentTeams: maxConcurrent,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(b)
}

func taskNodeIDsToStrings(ids []biz.TaskNodeID) []string {
	if len(ids) == 0 {
		return nil
	}
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = string(id)
	}
	return result
}

func assembleDAGTeams(ctx context.Context, assembler SpiritTeamAssemblerPort, dag *biz.TaskDAG, spiritSessionID, mode string, agentKeys []string, maxParallel, availableSlots int, autoStart bool) ([]AssembleTeamOutput, error) {
	var outputs []AssembleTeamOutput
	immediateStartCount := 0
	for _, node := range dag.OrderedNodes() {
		nodeAgentKeys := node.AgentKeys
		if len(nodeAgentKeys) == 0 {
			nodeAgentKeys = agentKeys
		}
		nodeMode := mode
		if node.Mode != "" {
			nodeMode = node.Mode
		}
		dependsOn := make([]string, len(node.DependsOn))
		for i, d := range node.DependsOn {
			dependsOn[i] = string(d)
		}
		canAutoStart := autoStart
		if canAutoStart && len(dependsOn) == 0 {
			if immediateStartCount >= availableSlots {
				canAutoStart = false
			} else {
				immediateStartCount++
			}
		}
		params := biz.SpiritTeamParams{
			SpiritSessionID:    spiritSessionID,
			TaskDescription:    node.Description,
			AgentKeys:          nodeAgentKeys,
			Mode:               nodeMode,
			DagNodeID:          string(node.ID),
			DependsOn:          taskNodeIDsToStrings(node.DependsOn),
			ParallelConfigJSON: buildParallelConfigJSON(maxParallel),
			AutoStart:          canAutoStart,
		}
		team, session, err := assembler.AssembleTeam(ctx, params)
		if err != nil {
			return outputs, err
		}
		outputs = append(outputs, AssembleTeamOutput{
			TeamID:    team.ID,
			SessionID: session.ID,
			TeamName:  team.DisplayName,
		})
	}
	return outputs, nil
}

// NewAssessComplexityTool creates the deprecated assess_complexity tool.
// DEPRECATED: Use plan_and_execute instead. This tool delegates to TaskPlannerPort.Plan.
func NewAssessComplexityTool(engine *ComplexityRuleEngine, planner biz.TaskPlannerPort) *trpcfunction.FunctionTool[AssessComplexityInput, AssessComplexityOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input AssessComplexityInput) (AssessComplexityOutput, error) {
			// Delegate to new planner when available.
			if planner != nil {
				spiritSessionID := spiritSessionIDFromCtx(ctx)
				planInput := biz.PlanInput{
					UserMessage:     input.UserMessage,
					SpiritSessionID: spiritSessionID,
				}
				taskPlan, err := planner.Plan(ctx, planInput)
				if err != nil {
					// Fallback to rule engine on error.
					return assessComplexityFallback(engine, input), nil
				}
				return AssessComplexityOutput{
					Level:          string(taskPlan.ComplexityLevel),
					Reasoning:      taskPlan.StrategyReason,
					SuggestedPath:  string(taskPlan.Strategy),
					AvailableTools: complexityAvailableTools(ComplexityLevel(taskPlan.ComplexityLevel)),
				}, nil
			}

			return assessComplexityFallback(engine, input), nil
		},
		trpcfunction.WithName("assess_complexity"),
		trpcfunction.WithDescription("[DEPRECATED] 评估用户消息的任务复杂度。请改用 plan_and_execute 工具。"),
	)
}

func assessComplexityFallback(engine *ComplexityRuleEngine, input AssessComplexityInput) AssessComplexityOutput {
	level := engine.Assess(input.UserMessage)
	path := complexityLevelToPath(level)
	return AssessComplexityOutput{
		Level:          string(level),
		Reasoning:      engine.LastReasoning(),
		SuggestedPath:  path,
		AvailableTools: complexityAvailableTools(level),
	}
}

func complexityLevelToPath(level ComplexityLevel) string {
	switch level {
	case ComplexitySimple:
		return "direct_answer"
	case ComplexityModerate:
		return "single_butler"
	case ComplexityComplex:
		return "orchestrator"
	default:
		return "single_butler"
	}
}

func complexityAvailableTools(level ComplexityLevel) []string {
	switch level {
	case ComplexitySimple:
		return simpleAvailableTools
	case ComplexityModerate:
		return moderateAvailableTools
	case ComplexityComplex:
		return complexAvailableTools
	default:
		return moderateAvailableTools
	}
}
