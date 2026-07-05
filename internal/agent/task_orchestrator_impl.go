package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	araneagraph "aranea-agents/internal/graph"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"trpc.group/trpc-go/trpc-agent-go/graph"
)

var _ biz.TaskOrchestratorPort = (*TaskOrchestratorImpl)(nil)

// TaskOrchestratorImpl implements biz.TaskOrchestratorPort.
type TaskOrchestratorImpl struct {
	spiritUC        *biz.SpiritTeamUsecase
	assembler       tools.SpiritTeamAssemblerPort
	controller      tools.SpiritTeamControllerPort
	compiler        *DAGToGraphCompiler
	repo            biz.OrchestrationRepository
	matcher         biz.AgentMatcherPort
	deps            TRPCBuilderDeps
	synthesis       tools.SpiritSynthesisPort
	checkpointSaver graph.CheckpointSaver
	orchCache       *biz.OrchestrationCache
	perfRepo        biz.AgentPerformanceRepository
	evolutionSugg   biz.EvolutionSuggestionRepo
	eventBus        biz.EventBus
	nl2graph        araneagraph.NL2GraphConverter
	lg              loggateway.Logger
}

// NewTaskOrchestratorImpl creates a new TaskOrchestratorImpl.
// The nl2graph parameter may be nil when NL2Graph conversion is not configured;
// in that case orchestrateDAG falls back to the sequential pipeline when no
// pre-defined graph template (TaskDAG) is available.
func NewTaskOrchestratorImpl(
	spiritUC *biz.SpiritTeamUsecase,
	assembler tools.SpiritTeamAssemblerPort,
	controller tools.SpiritTeamControllerPort,
	compiler *DAGToGraphCompiler,
	repo biz.OrchestrationRepository,
	matcher biz.AgentMatcherPort,
	deps TRPCBuilderDeps,
	synthesis tools.SpiritSynthesisPort,
	checkpointSaver graph.CheckpointSaver,
	orchCache *biz.OrchestrationCache,
	perfRepo biz.AgentPerformanceRepository,
	evolutionSugg biz.EvolutionSuggestionRepo,
	eventBus biz.EventBus,
	nl2graph araneagraph.NL2GraphConverter,
	lg loggateway.Logger,
) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		spiritUC:        spiritUC,
		assembler:       assembler,
		controller:      controller,
		compiler:        compiler,
		repo:            repo,
		matcher:         matcher,
		deps:            deps,
		synthesis:       synthesis,
		checkpointSaver: checkpointSaver,
		orchCache:       orchCache,
		perfRepo:        perfRepo,
		evolutionSugg:   evolutionSugg,
		eventBus:        eventBus,
		nl2graph:        nl2graph,
		lg:              lg,
	}
}

// Orchestrate builds and executes the orchestration graph based on the TaskPlan and AllocationPlan.
//
// Deprecated: 2026-07-05 Step 5 — team 创建已迁移到 PlanExecutor + RealTeamOrchestrator。
// plan_and_execute 工具的 executeOrchestratePhase 不再调用此方法（返回 placeholder handle）。
// PlanExecutor 订阅 PlanBoardCreatedEvent 触发 RealTeamOrchestrator.Orchestrate，
// 使用 PlanStep.AgentKeys 组建 team，严格 DAG 调度。
// 此方法保留以兼容 TaskOrchestratorPort 接口（CheckProgress/Cancel/RecoverAllInterrupted 仍被使用），
// 但 orchestrateDAG/orchestrateTeam/orchestrateParallelTeams 路径已成为死代码。
func (o *TaskOrchestratorImpl) Orchestrate(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan) (handle *biz.OrchestrationHandle, err error) {
	start := time.Now()
	defer func() {
		metrics.SpiritOrchDuration.Observe(time.Since(start).Seconds())
	}()

	// Start orchestration phase span (P3-2): Trace propagation across Spirit→Team→Graph.
	bridge := turntrace.FromContext(ctx)
	if bridge != nil {
		ctx, _ = bridge.StartPhase(ctx, turntrace.PhaseOrch)
	}
	defer func() {
		if bridge != nil {
			bridge.EndPhase(turntrace.PhaseOrch, err)
		}
	}()

	if taskPlan == nil {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "task_plan is required")
	}
	if allocPlan == nil {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "allocation_plan is required")
	}

	o.lg.Info("TaskOrchestrator: starting orchestration",
		loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
		loggateway.Str("task_plan_id", taskPlan.ID),
		loggateway.Str("strategy", string(taskPlan.Strategy)),
		loggateway.Str("spirit_session_id", taskPlan.SpiritSessionID),
	)

	// Create OrchestrationHandle with status "pending".
	handle = &biz.OrchestrationHandle{
		ID:              "orch_" + uuid.NewString()[:12],
		TaskPlanID:      taskPlan.ID,
		AllocationID:    allocPlan.ID,
		SpiritSessionID: taskPlan.SpiritSessionID,
		TraceID:         taskPlan.TraceID,
		Strategy:        taskPlan.Strategy,
		Status:          biz.OrchestrationStatusPending,
	}

	// Save initial checkpoint for the orchestration lineage.
	o.saveInitialCheckpoint(ctx, handle)

	// Execute based on strategy.
	switch taskPlan.Strategy {
	case biz.StrategyDirect:
		// No orchestration needed, Spirit answers directly.
		// AS-FSM-01: validate state transition before assignment.
		o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusCompleted)
		o.lg.Info("TaskOrchestrator: direct strategy, no orchestration needed",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
		)

	case biz.StrategySingleAgent:
		// Agent-as-Tool path.
		if err := o.orchestrateSingleAgent(ctx, taskPlan, allocPlan, handle); err != nil {
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			// Best-effort persist on error path; primary error is returned below.
			_ = o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyParallel:
		// Parallel team path.
		if err := o.orchestrateTeam(ctx, taskPlan, allocPlan, handle, "parallel"); err != nil {
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			_ = o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyDAG:
		// DAG → Graph compilation path.
		if err := o.orchestrateDAG(ctx, taskPlan, allocPlan, handle); err != nil {
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			_ = o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyCoordinator:
		// Coordinator team path.
		if err := o.orchestrateTeam(ctx, taskPlan, allocPlan, handle, "coordinator"); err != nil {
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			_ = o.persistHandle(ctx, handle)
			return nil, err
		}

	default:
		return nil, apierror.BadRequest(apierror.DomainSpirit, "unknown orchestration strategy: %s", taskPlan.Strategy)
	}

	// Save a step checkpoint after orchestration setup completes.
	o.saveStepCheckpoint(ctx, handle, "orchestrate_setup")

	// AS-EVT-01 (Important): persist the handle BEFORE publishing the event.
	// If persistence fails, the event must not be published — otherwise
	// consumers would receive a started event for an orchestration that
	// does not exist in the store, breaking Recover/CheckProgress.
	persisted, err := o.repo.Create(ctx, handle)
	if err != nil {
		// Red line #22: do not swallow the error. Return it so the caller
		// can decide how to handle the persistence failure.
		o.lg.Warn("TaskOrchestrator: failed to persist orchestration handle",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainSpirit, "persist orchestration handle").WithCause(err)
	}
	handle = persisted

	// Publish spirit_orchestration_started event after successful persistence.
	o.publishOrchestrationStarted(ctx, handle, taskPlan)

	return handle, nil
}

// orchestrateSingleAgent handles the Agent-as-Tool path.
func (o *TaskOrchestratorImpl) orchestrateSingleAgent(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle) error {
	o.lg.Info("TaskOrchestrator: single-agent path",
		loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
		loggateway.Str("orchestration_id", handle.ID),
	)

	// Build the Agent-as-Tool.
	taskDesc := taskPlan.UserMessage
	var capabilities []string
	if len(allocPlan.Allocations) > 0 {
		taskDesc = allocPlan.Allocations[0].SubTaskName
		if allocPlan.Allocations[0].SubTaskName == "" {
			taskDesc = allocPlan.Allocations[0].SubTaskID
		}
	}

	_, err := BuildAgentAsTool(ctx, o.matcher, o.deps, o.lg, taskDesc, capabilities)
	if err != nil {
		return apierror.Internal(apierror.DomainSpirit, "build agent-as-tool").WithCause(err)
	}

	// For now, mark as running. The actual tool invocation happens in the Spirit turn.
	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning)
	// Extract agent key from allocation for performance tracking.
	if len(allocPlan.Allocations) > 0 {
		key := strings.TrimSpace(allocPlan.Allocations[0].AssignedKey)
		if key != "" {
			handle.AgentKeys = []string{key}
		}
	}
	o.lg.Info("TaskOrchestrator: agent-as-tool built successfully",
		loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
		loggateway.Str("orchestration_id", handle.ID),
	)
	return nil
}

// orchestrateTeam handles the parallel/coordinator team path.
// For parallel strategy, each subtask allocation creates an independent team
// so that tasks with different descriptions can execute concurrently.
// For coordinator strategy, all agents are placed in a single team with a
// coordinator agent that delegates.
func (o *TaskOrchestratorImpl) orchestrateTeam(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle, mode string) error {
	o.lg.Info("TaskOrchestrator: team path",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("mode", mode),
	)

	// Collect agent keys from allocations.
	agentKeys := make([]string, 0, len(allocPlan.Allocations))
	for _, alloc := range allocPlan.Allocations {
		key := strings.TrimSpace(alloc.AssignedKey)
		if key != "" {
			agentKeys = append(agentKeys, key)
		}
	}

	if len(agentKeys) == 0 {
		return apierror.BadRequest(apierror.DomainSpirit, "no agent keys in allocation plan")
	}

	// Sort agents by historical performance for this task type.
	agentKeys = o.sortByPerformance(ctx, agentKeys, string(taskPlan.Strategy))

	var teamIDs []string

	if mode == "parallel" && len(allocPlan.Allocations) > 1 {
		// Parallel strategy: create one team per subtask allocation.
		// Each team gets its own task description from the subtask,
		// enabling independent execution with focused instructions.
		parallelTeamIDs, parallelErr := o.orchestrateParallelTeams(ctx, taskPlan, allocPlan)
		if parallelErr != nil {
			return parallelErr
		}
		teamIDs = parallelTeamIDs
	} else {
		// Coordinator strategy (or single allocation): single team with all agents.
		// Set DagNodeID to the first allocation's SubTaskID so updatePlanStepForTeam
		// can match and update the corresponding plan step when the team completes.
		dagNodeID := ""
		teamName := ""
		if len(allocPlan.Allocations) > 0 {
			dagNodeID = allocPlan.Allocations[0].SubTaskID
			// 2026-07-04 问题 2 修复：透传 SubTaskName 作为团队展示名。
			teamName = allocPlan.Allocations[0].SubTaskName
		}
		params := biz.SpiritTeamParams{
			SpiritSessionID: taskPlan.SpiritSessionID,
			TaskDescription: taskPlan.UserMessage,
			AgentKeys:       agentKeys,
			Mode:            mode,
			AutoStart:       true,
			DagNodeID:       dagNodeID,
			TeamName:        teamName,
		}

		team, _, _, err := o.assembler.AssembleTeam(ctx, params)
		if err != nil {
			return apierror.Internal(apierror.DomainSpirit, "assemble team").WithCause(err)
		}
		teamIDs = []string{team.ID}

		o.lg.Info("TaskOrchestrator: team assembled and started",
			loggateway.StepID(biz.SpiritStepOrchestratorExecute),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("team_id", team.ID),
		)
	}

	handle.TeamIDs = teamIDs
	handle.AgentKeys = agentKeys
	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning)
	return nil
}

// orchestrateParallelTeams creates one independent team per subtask allocation.
// Each team receives its own task description derived from the subtask name,
// enabling focused and concurrent execution. Teams that fail to assemble are
// logged but do not block the orchestration — the remaining teams proceed.
// Returns an error only if ALL teams fail to assemble.
//
// Teams are assembled concurrently using errgroup. A pre-allocated results
// slice avoids shared-slice concurrent writes (red line #21). Each goroutine
// returns nil to errgroup so a single assembly failure does NOT cancel other
// in-flight assemblies, preserving the fault-tolerance semantics of the
// previous serial implementation.
func (o *TaskOrchestratorImpl) orchestrateParallelTeams(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan) ([]string, error) {
	allocs := allocPlan.Allocations
	if len(allocs) == 0 {
		return nil, nil
	}

	// Pre-allocate results slice: each goroutine writes only to its own index.
	results := make([]parallelTeamResult, len(allocs))

	// errgroup.WithContext creates a child context for cancellation propagation.
	// Goroutines return nil so a single failure does not cancel siblings.
	g, gctx := errgroup.WithContext(ctx)
	for i, alloc := range allocs {
		idx, al := i, alloc
		g.Go(func() error {
			key := strings.TrimSpace(al.AssignedKey)
			if key == "" {
				// Skip allocations without an agent key; leave result zero-valued.
				return nil
			}

			// Use the subtask name as the team's task description for focused execution.
			// 2026-07-04 问题 2 修复：TaskDescription 不再回退到 SubTaskID（st_1），
			// 改为回退到 UserMessage；TeamName 单独传递 SubTaskName 用于展示。
			taskDesc := al.SubTaskName
			if taskDesc == "" {
				taskDesc = taskPlan.UserMessage
			}
			teamName := al.SubTaskName
			if teamName == "" {
				teamName = taskPlan.UserMessage
			}

			params := biz.SpiritTeamParams{
				SpiritSessionID: taskPlan.SpiritSessionID,
				TaskDescription: taskDesc,
				AgentKeys:       []string{key},
				Mode:            "parallel",
				AutoStart:       true,
				DagNodeID:       al.SubTaskID,
				TeamName:        teamName,
			}

			team, _, _, err := o.assembler.AssembleTeam(gctx, params)
			if err != nil {
				results[idx] = parallelTeamResult{
					subtaskID: al.SubTaskID,
					agentKey:  key,
					err:       err,
				}
				o.lg.Warn("TaskOrchestrator: failed to assemble team for subtask, skipping",
					loggateway.StepID(biz.SpiritStepOrchestratorExecute),
					loggateway.Str("subtask_id", al.SubTaskID),
					loggateway.Str("agent_key", key),
					loggateway.Err(err),
				)
				return nil
			}

			results[idx] = parallelTeamResult{
				teamID:    team.ID,
				subtaskID: al.SubTaskID,
				agentKey:  key,
			}
			o.lg.Info("TaskOrchestrator: parallel team assembled for subtask",
				loggateway.StepID(biz.SpiritStepOrchestratorExecute),
				loggateway.Str("team_id", team.ID),
				loggateway.Str("subtask_id", al.SubTaskID),
				loggateway.Str("agent_key", key),
			)
			return nil
		})
	}
	// All goroutines return nil, so g.Wait() always returns nil. The error
	// channel is intentionally unused — failures are captured in `results`.
	_ = g.Wait()

	// Collect successful team IDs and track the last non-nil error.
	var teamIDs []string
	var lastErr error
	for _, r := range results {
		if r.err != nil {
			lastErr = r.err
			continue
		}
		if r.teamID != "" {
			teamIDs = append(teamIDs, r.teamID)
		}
	}

	if len(teamIDs) == 0 {
		return nil, apierror.Internal(apierror.DomainSpirit, "all parallel teams failed to assemble").WithCause(lastErr)
	}

	return teamIDs, nil
}

// parallelTeamResult captures the outcome of a single parallel team assembly.
// Each goroutine writes its own instance into a pre-allocated slice slot.
type parallelTeamResult struct {
	teamID    string
	subtaskID string
	agentKey  string
	err       error
}

// orchestrateDAG handles the DAG → multi-team path.
//
// In the three-mode system (direct/parallel/dag), dag mode creates N teams —
// one per allocation/subtask — where each team has ≥2 members collaborating.
// This mirrors orchestrateParallelTeams (concurrent assembly, fault-tolerant)
// but each team carries multiple agents (lead + TeamMemberKeys from the
// allocator) instead of a single agent.
//
// Inter-team dependencies from taskPlan.TaskDAG are passed via
// SpiritTeamParams.DependsOn so the runtime can sequence teams when needed.
// When TaskDAG is nil (no pre-defined template), the function falls back to
// orchestrateWithoutTemplate for NL2Graph conversion.
func (o *TaskOrchestratorImpl) orchestrateDAG(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle) error {
	o.lg.Info("TaskOrchestrator: DAG path (multi-team)",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Int("allocations", len(allocPlan.Allocations)),
	)

	// Collect all agent keys for the handle (for status tracking).
	allAgentKeys := make([]string, 0, len(allocPlan.Allocations)*2)
	for _, alloc := range allocPlan.Allocations {
		if k := strings.TrimSpace(alloc.AssignedKey); k != "" {
			allAgentKeys = append(allAgentKeys, k)
		}
		allAgentKeys = append(allAgentKeys, alloc.TeamMemberKeys...)
	}
	if len(allAgentKeys) == 0 {
		return apierror.BadRequest(apierror.DomainSpirit, "no agent keys in allocation plan for DAG strategy")
	}

	// No subtasks and no DAG template: fall back to NL2Graph or sequential
	// pipeline (legacy single-team path). This preserves backward compatibility
	// for plans that reach dag strategy without decomposition (rare).
	if len(allocPlan.Allocations) == 0 || (taskPlan.TaskDAG == nil && len(allocPlan.Allocations) <= 1) {
		return o.orchestrateWithoutTemplate(ctx, taskPlan, allocPlan, handle, allAgentKeys)
	}

	// Build subtask → dependsOn lookup from the DAG (if available).
	dependsMap := make(map[string][]string)
	if taskPlan.TaskDAG != nil {
		for _, node := range taskPlan.TaskDAG.Nodes {
			if len(node.DependsOn) > 0 {
				dependsMap[node.ID] = node.DependsOn
			}
		}
	}

	// Create 1 team per allocation concurrently (following the
	// orchestrateParallelTeams pattern). Each team gets the lead agent
	// (AssignedKey) plus any TeamMemberKeys as its members.
	allocs := allocPlan.Allocations
	results := make([]parallelTeamResult, len(allocs))
	g, gctx := errgroup.WithContext(ctx)
	for i, alloc := range allocs {
		idx, al := i, alloc
		g.Go(func() error {
			lead := strings.TrimSpace(al.AssignedKey)
			if lead == "" {
				return nil // skip allocations without a lead agent
			}

			// Build the team's agent list: lead + additional members.
			agentKeys := []string{lead}
			agentKeys = append(agentKeys, al.TeamMemberKeys...)

			// 2026-07-04 问题 2 修复：TaskDescription 不再回退到 SubTaskID（st_1），
			// 改为回退到 UserMessage；TeamName 单独传递 SubTaskName 用于展示。
			taskDesc := al.SubTaskName
			if taskDesc == "" {
				taskDesc = taskPlan.UserMessage
			}
			teamName := al.SubTaskName
			if teamName == "" {
				teamName = taskPlan.UserMessage
			}

			params := biz.SpiritTeamParams{
				SpiritSessionID: taskPlan.SpiritSessionID,
				TaskDescription: taskDesc,
				AgentKeys:       agentKeys,
				Mode:            "coordinator", // multi-member team collaboration
				AutoStart:       true,
				DagNodeID:       al.SubTaskID,
				DependsOn:       dependsMap[al.SubTaskID],
				TeamName:        teamName,
			}

			team, _, _, err := o.assembler.AssembleTeam(gctx, params)
			if err != nil {
				o.lg.Warn("TaskOrchestrator: DAG team assembly failed",
					loggateway.StepID(biz.SpiritStepOrchestratorExecute),
					loggateway.Str("orchestration_id", handle.ID),
					loggateway.Str("sub_task_id", al.SubTaskID),
					loggateway.Str("lead", lead),
					loggateway.Err(err),
				)
				results[idx] = parallelTeamResult{err: err}
				return nil // don't cancel sibling teams
			}

			o.lg.Info("TaskOrchestrator: DAG team assembled",
				loggateway.StepID(biz.SpiritStepOrchestratorExecute),
				loggateway.Str("orchestration_id", handle.ID),
				loggateway.Str("team_id", team.ID),
				loggateway.Str("sub_task_id", al.SubTaskID),
				loggateway.Int("member_count", len(agentKeys)),
			)
			results[idx] = parallelTeamResult{
				teamID:    team.ID,
				subtaskID: al.SubTaskID,
				agentKey:  lead,
			}
			return nil
		})
	}
	_ = g.Wait()

	// Collect successfully assembled team IDs. If ALL teams failed, return error.
	var teamIDs []string
	var failedCount int
	for _, r := range results {
		if r.teamID != "" {
			teamIDs = append(teamIDs, r.teamID)
		} else if r.err != nil {
			failedCount++
		}
	}
	if len(teamIDs) == 0 {
		return apierror.Internal(apierror.DomainSpirit, "all DAG team assemblies failed (%d teams)", failedCount)
	}

	handle.TeamIDs = teamIDs
	handle.AgentKeys = allAgentKeys
	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning)

	o.lg.Info("TaskOrchestrator: DAG multi-team assembly complete",
		loggateway.StepID(biz.SpiritStepOrchestratorExecute),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Int("team_count", len(teamIDs)),
		loggateway.Int("failed_count", failedCount),
	)
	return nil
}

// orchestrateWithoutTemplate handles the DAG strategy path when no pre-defined
// graph template (TaskDAG) is available. It attempts NL2Graph conversion to
// generate a GraphBuildConfig from the task description; on failure (converter
// not wired or conversion error), it falls back to the existing sequential
// pipeline by delegating to orchestrateTeam with "parallel" mode.
func (o *TaskOrchestratorImpl) orchestrateWithoutTemplate(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle, agentKeys []string) error {
	cfg, convErr := o.tryNL2GraphConversion(ctx, taskPlan, allocPlan, handle)
	if convErr != nil || cfg == nil {
		o.lg.Warn("TaskOrchestrator: NL2Graph conversion unavailable, falling back to sequential pipeline",
			loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(convErr),
		)
		return o.orchestrateTeam(ctx, taskPlan, allocPlan, handle, "parallel")
	}

	// Determine mode from the NL2Graph-generated config structure.
	mode := "coordinator"
	if len(cfg.Edges) == 0 && len(cfg.Nodes) > 1 {
		mode = "parallel"
	}

	params := biz.SpiritTeamParams{
		SpiritSessionID: taskPlan.SpiritSessionID,
		TaskDescription: taskPlan.UserMessage,
		AgentKeys:       agentKeys,
		Mode:            mode,
		AutoStart:       true,
		// 2026-07-04 问题 2 修复：NL2Graph 路径无 subtask name，用 UserMessage 作为团队展示名。
		TeamName: taskPlan.UserMessage,
	}
	team, _, _, err := o.assembler.AssembleTeam(ctx, params)
	if err != nil {
		return apierror.Internal(apierror.DomainSpirit, "assemble NL2Graph team").WithCause(err)
	}
	handle.TeamIDs = []string{team.ID}
	handle.AgentKeys = agentKeys
	handle.GraphExecutionID = team.ID
	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning)
	o.lg.Info("TaskOrchestrator: NL2Graph team assembled",
		loggateway.StepID(biz.SpiritStepOrchestratorExecute),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("team_id", team.ID),
	)
	return nil
}

// tryNL2GraphConversion attempts to convert the task description into a
// GraphBuildConfig using the NL2GraphConverter. Returns nil config (with nil
// error) when the converter is not wired; returns an error when conversion
// fails. In both cases the caller should fall back to the sequential pipeline.
func (o *TaskOrchestratorImpl) tryNL2GraphConversion(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle) (*biz.GraphBuildConfig, error) {
	if o.nl2graph == nil {
		return nil, nil
	}
	availableAgents := buildAgentCapabilitiesFromAlloc(allocPlan)
	cfg, err := o.nl2graph.Convert(ctx, taskPlan.UserMessage, availableAgents)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	o.lg.Info("TaskOrchestrator: NL2Graph conversion succeeded",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Int("node_count", len(cfg.Nodes)),
		loggateway.Int("edge_count", len(cfg.Edges)),
	)
	return cfg, nil
}

// buildAgentCapabilitiesFromAlloc builds a minimal AgentCapability list from
// the AllocationPlan for NL2Graph conversion. Only AgentKey and DisplayName
// are populated because the orchestrator does not have access to the full
// agent catalog; NL2Graph uses these as fallback agent selectors.
func buildAgentCapabilitiesFromAlloc(allocPlan *biz.AllocationPlan) []biz.AgentCapability {
	if allocPlan == nil {
		return nil
	}
	caps := make([]biz.AgentCapability, 0, len(allocPlan.Allocations))
	for _, alloc := range allocPlan.Allocations {
		key := strings.TrimSpace(alloc.AssignedKey)
		if key == "" {
			continue
		}
		caps = append(caps, biz.AgentCapability{
			AgentKey:    key,
			DisplayName: alloc.AssignedName,
		})
	}
	return caps
}

// CheckProgress returns the progress of each subtask in the orchestration.
func (o *TaskOrchestratorImpl) CheckProgress(ctx context.Context, orchestrationID string) ([]biz.TaskProgress, error) {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	// For team-based strategies, delegate to team progress checking.
	if len(handle.TeamIDs) > 0 && o.controller != nil {
		teamProgresses, err := o.controller.CheckTeamProgress(ctx, handle.SpiritSessionID)
		if err != nil {
			return nil, err
		}
		// Convert TeamProgress to TaskProgress.
		out := make([]biz.TaskProgress, 0, len(teamProgresses))
		for _, tp := range teamProgresses {
			out = append(out, biz.TaskProgress{
				SubTaskName: tp.TeamName,
				Status:      tp.Status,
				Progress:    tp.ProgressPct / 100.0,
			})
		}
		return out, nil
	}

	// For direct/single-agent strategies, return based on handle status.
	progress := 0.0
	status := string(handle.Status)
	switch handle.Status {
	case biz.OrchestrationStatusCompleted:
		progress = 1.0
	case biz.OrchestrationStatusRunning:
		progress = 0.5
	}
	return []biz.TaskProgress{{
		Status:   status,
		Progress: progress,
	}}, nil
}

// Cancel cancels the orchestration and all associated teams.
func (o *TaskOrchestratorImpl) Cancel(ctx context.Context, orchestrationID string) error {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusPending && handle.Status != biz.OrchestrationStatusRunning {
		return apierror.BadRequest(apierror.DomainSpirit, "only pending or running orchestrations can be cancelled")
	}

	// Cancel all teams.
	for _, teamID := range handle.TeamIDs {
		if o.controller != nil {
			if cancelErr := o.controller.CancelTeam(ctx, teamID); cancelErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to cancel team",
					loggateway.StepID(biz.SpiritStepOrchestratorExecute),
					loggateway.Str("team_id", teamID),
					loggateway.Err(cancelErr),
				)
			}
		}
	}

	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusCancelled)
	_, err = o.repo.Update(ctx, handle)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to update cancelled orchestration",
			loggateway.StepID(biz.SpiritStepOrchestratorExecute),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Err(err),
		)
	}

	// Publish spirit_orchestration_interrupted event.
	o.publishOrchestrationInterrupted(ctx, handle)

	return nil
}

// Synthesize synthesizes the results of the orchestration and persists the result.
func (o *TaskOrchestratorImpl) Synthesize(ctx context.Context, orchestrationID string) (*biz.SynthesisOutput, error) {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.SpiritSessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "orchestration has no spirit_session_id")
	}

	// Delegate to the SpiritSynthesisService.
	if o.synthesis != nil {
		output, err := o.synthesis.SynthesizeResults(ctx, handle.SpiritSessionID, "")
		if err != nil {
			return nil, err
		}
		// Persist synthesis result to handle so it survives process restarts.
		if output != nil {
			synthesisJSON, marshalErr := marshalSynthesisOutput(output)
			if marshalErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to marshal synthesis result",
					loggateway.StepID(biz.SpiritStepOrchestratorSynthesize),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(marshalErr),
				)
			} else {
				handle.SynthesisResultJSON = synthesisJSON
				o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusCompleted)
				if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
					o.lg.Warn("TaskOrchestrator: failed to update orchestration with synthesis result",
						loggateway.StepID(biz.SpiritStepOrchestratorSynthesize),
						loggateway.Str("orchestration_id", orchestrationID),
						loggateway.Err(updateErr),
					)
				}
			}

			// Online learning loop: update cache and performance after synthesis
			o.learnFromOrchestration(ctx, handle, output)
		}
		return output, nil
	}

	return nil, apierror.Internal(apierror.DomainSpirit, "synthesis service not available")
}

// Recover recovers an interrupted orchestration from its last checkpoint.
func (o *TaskOrchestratorImpl) Recover(ctx context.Context, orchestrationID string) error {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusInterrupted {
		return apierror.BadRequest(apierror.DomainSpirit, "only interrupted orchestrations can be recovered (status: %s)", handle.Status)
	}

	o.lg.Info("TaskOrchestrator: recovering orchestration",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
		loggateway.Str("checkpoint_id", handle.CheckpointID),
	)

	// If no checkpoint is available, we cannot recover the graph state.
	// Mark as failed since there is no way to resume.
	if handle.CheckpointID == "" {
		o.lg.Warn("TaskOrchestrator: no checkpoint available, marking orchestration as failed",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
		)
		o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
		if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
			o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Err(updateErr),
			)
		}
		return apierror.NotFound(apierror.DomainSpirit, "no checkpoint available for orchestration %s", orchestrationID)
	}

	// Attempt to load the latest checkpoint from the CheckpointSaver.
	if o.checkpointSaver != nil {
		lineageID := handle.ID
		config := graph.CreateCheckpointConfig(lineageID, handle.CheckpointID, "")
		tuple, loadErr := o.checkpointSaver.GetTuple(ctx, config)
		if loadErr != nil {
			o.lg.Warn("TaskOrchestrator: failed to load checkpoint",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Str("checkpoint_id", handle.CheckpointID),
				loggateway.Err(loadErr),
			)
			// Cannot load checkpoint; mark as failed.
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
					loggateway.StepID(biz.SpiritStepOrchestratorRecover),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(updateErr),
				)
			}
			return apierror.Internal(apierror.DomainSpirit, "failed to load checkpoint for orchestration %s", orchestrationID).WithCause(loadErr)
		}

		if tuple == nil || tuple.Checkpoint == nil {
			o.lg.Warn("TaskOrchestrator: checkpoint not found, marking orchestration as failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Str("checkpoint_id", handle.CheckpointID),
			)
			o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed)
			if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
					loggateway.StepID(biz.SpiritStepOrchestratorRecover),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(updateErr),
				)
			}
			return apierror.NotFound(apierror.DomainSpirit, "checkpoint %s not found for orchestration %s", handle.CheckpointID, orchestrationID)
		}

		o.lg.Info("TaskOrchestrator: checkpoint loaded successfully",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Str("checkpoint_id", tuple.Checkpoint.ID),
		)

		// Rebuild GraphAgent state from checkpoint: validate critical fields
		// and prepare the handle for graph runtime resumption.
		if err := o.restoreGraphFromCheckpoint(ctx, handle, tuple); err != nil {
			o.lg.Warn("TaskOrchestrator: GraphAgent rebuild from checkpoint failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Err(err),
			)
			// Non-fatal: the checkpoint was loaded but state validation failed.
			// Continue recovery so the orchestration can at least be tracked.
		}
	}

	// Mark as running so the orchestration can be tracked.
	o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning)
	_, err = o.repo.Update(ctx, handle)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to update recovered orchestration",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Err(err),
		)
		return err
	}

	o.lg.Info("TaskOrchestrator: orchestration recovered",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
	)
	return nil
}

// restoreGraphFromCheckpoint rebuilds GraphAgent state from a loaded checkpoint.
// It validates that critical state fields in the checkpoint match the orchestration
// handle and updates the handle's checkpoint ID for graph runtime resumption.
func (o *TaskOrchestratorImpl) restoreGraphFromCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle, tuple *graph.CheckpointTuple) error {
	if tuple == nil || tuple.Checkpoint == nil {
		return apierror.Internal(apierror.DomainSpirit, "checkpoint tuple is nil")
	}

	ckpt := tuple.Checkpoint
	values := ckpt.ChannelValues

	// Validate critical state fields match the handle.
	if orchID, ok := values["orchestration_id"].(string); ok && orchID != "" && orchID != handle.ID {
		o.lg.Warn("Checkpoint orchestration_id 与 handle 不匹配",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("handle_id", handle.ID),
			loggateway.Str("checkpoint_orchestration_id", orchID),
		)
	}

	if sessionID, ok := values["spirit_session_id"].(string); ok && sessionID != "" && sessionID != handle.SpiritSessionID {
		o.lg.Warn("Checkpoint spirit_session_id 与 handle 不匹配",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("handle_session_id", handle.SpiritSessionID),
			loggateway.Str("checkpoint_session_id", sessionID),
		)
	}

	// Restore strategy from checkpoint if handle is missing it.
	if handle.Strategy == "" {
		if strategy, ok := values["strategy"].(string); ok && strategy != "" {
			handle.Strategy = biz.OrchestrationStrategy(strategy)
		}
	}

	// Update checkpoint ID to the latest loaded checkpoint so the graph runtime
	// can resume from this exact point when the team runner restarts.
	handle.CheckpointID = ckpt.ID

	o.lg.Info("GraphAgent 状态已从 checkpoint 重建",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("checkpoint_id", ckpt.ID),
		loggateway.Str("strategy", string(handle.Strategy)),
		loggateway.Int("channel_count", len(values)),
	)

	return nil
}

// RecoverAllInterrupted finds all interrupted orchestrations and attempts recovery.
// TODO(debt): DEV-07 — Phase 1/2 interruption recovery not implemented. Only OrchestrationHandle
// is recovered, not draft TaskPlan/AllocationPlan.
// See: https://github.com/aranea-agents/aranea-agents/issues/DEV-07
func (o *TaskOrchestratorImpl) RecoverAllInterrupted(ctx context.Context) error {
	handles, err := o.repo.ListByStatus(ctx, biz.OrchestrationStatusInterrupted)
	if err != nil {
		return apierror.Internal(apierror.DomainSpirit, "list interrupted orchestrations").WithCause(err)
	}

	if len(handles) == 0 {
		return nil
	}

	o.lg.Info("TaskOrchestrator: recovering interrupted orchestrations",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Int("count", len(handles)),
	)

	var failedCount int
	for _, h := range handles {
		if err := o.Recover(ctx, h.ID); err != nil {
			failedCount++
			o.lg.Warn("TaskOrchestrator: failed to recover orchestration",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", h.ID),
				loggateway.Err(err),
			)
			continue
		}
	}

	if failedCount > 0 {
		o.lg.Warn("TaskOrchestrator: some orchestrations failed to recover",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Int("total", len(handles)),
			loggateway.Int("failed", failedCount),
		)
	}
	return nil
}

// learnFromOrchestration updates OrchestrationCache and AgentPerformance after
// an orchestration completes, implementing the online learning loop (T3.5).
//
// NOTE: 非原子操作，允许部分成功（在线学习侧效）。三个独立更新（OrchestrationCache、
// AgentPerformance、EvolutionSuggestion）各自独立持久化，失败仅 Warn 不回滚。
// 这是有意设计：每个更新独立有意义，部分失败不影响其他维度的学习信号。
func (o *TaskOrchestratorImpl) learnFromOrchestration(ctx context.Context, handle *biz.OrchestrationHandle, synthesis *biz.SynthesisOutput) {
	if handle == nil || synthesis == nil {
		return
	}

	dqScore := computeDQScoreFromSynthesis(synthesis)

	o.lg.Info("在线学习: 更新编排缓存和 Agent 性能",
		loggateway.StepID(biz.SpiritStepOrchestratorLearn),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("strategy", string(handle.Strategy)),
		loggateway.Float64("dq_score", dqScore),
	)

	// 1. Update OrchestrationCache with DQ score
	topology := biz.TopologyCoordinator
	switch handle.Strategy {
	case biz.StrategyDirect:
		topology = biz.TopologyDirect
	case biz.StrategyParallel:
		topology = biz.TopologyParallel
	case biz.StrategyDAG:
		topology = biz.TopologyHybrid
	}
	if o.orchCache != nil {
		taskPattern := biz.ExtractTaskPattern(handle.ID) // Use orchestration ID as pattern key
		agentKeys := extractAgentKeysFromHandle(handle)
		o.orchCache.RecordCompletionWithAgents(ctx, taskPattern, topology, dqScore, len(handle.TeamIDs), 0, agentKeys)
		o.lg.Info("在线学习: 编排缓存已更新",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("task_pattern", taskPattern),
			loggateway.Float64("dq_score", dqScore),
		)
	}

	// 2. Generate evolution suggestion when DQ Score is low
	if dqScore < biz.DQEvolutionThreshold && o.evolutionSugg != nil {
		o.maybeCreateEvolutionSuggestion(ctx, handle, dqScore, topology)
	}

	// 3. Update AgentPerformance for each agent in the orchestration
	if o.perfRepo != nil {
		successCount := 0
		if dqScore >= 0.5 {
			successCount = 1
		}
		agentKeys := extractAgentKeysFromHandle(handle)
		for _, agentKey := range agentKeys {
			existing, err := o.perfRepo.Get(ctx, agentKey, string(handle.Strategy))
			if err != nil || existing == nil {
				// New performance record
				perf := &biz.AgentPerformance{
					AgentKey:       agentKey,
					TaskType:       string(handle.Strategy),
					TotalRuns:      1,
					SuccessRuns:    successCount,
					SuccessRate:    float64(successCount),
					AvgDQScore:     dqScore,
					LastExecutedAt: time.Now().UTC().Format(time.RFC3339),
				}
				if upsertErr := o.perfRepo.Upsert(ctx, perf); upsertErr != nil {
					o.lg.Warn("在线学习: AgentPerformance 更新失败",
						loggateway.StepID(biz.SpiritStepOrchestratorLearn),
						loggateway.Str("agent_key", agentKey),
						loggateway.Err(upsertErr),
					)
				}
			} else {
				// Update existing performance record with running average
				existing.TotalRuns++
				existing.SuccessRuns += successCount
				existing.SuccessRate = float64(existing.SuccessRuns) / float64(existing.TotalRuns)
				existing.AvgDQScore = (existing.AvgDQScore*float64(existing.TotalRuns-1) + dqScore) / float64(existing.TotalRuns)
				existing.LastExecutedAt = time.Now().UTC().Format(time.RFC3339)
				if upsertErr := o.perfRepo.Upsert(ctx, existing); upsertErr != nil {
					o.lg.Warn("在线学习: AgentPerformance 更新失败",
						loggateway.StepID(biz.SpiritStepOrchestratorLearn),
						loggateway.Str("agent_key", agentKey),
						loggateway.Err(upsertErr),
					)
				}
			}
		}
		o.lg.Info("在线学习: Agent 性能已更新",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Int("agent_count", len(agentKeys)),
		)
	}
}

// maybeCreateEvolutionSuggestion generates an orchestration_optimization evolution suggestion
// when DQ Score is below the evolution threshold. It performs dedup by checking pending
// suggestions for the same agentID + type + title combination.
func (o *TaskOrchestratorImpl) maybeCreateEvolutionSuggestion(ctx context.Context, handle *biz.OrchestrationHandle, dqScore float64, topology biz.TopologyType) {
	// Use SpiritSessionID as the evolution target — it represents the spirit session
	// that owns this orchestration, enabling cross-orchestration dedup within the same session.
	targetID := handle.SpiritSessionID
	if targetID == "" {
		targetID = handle.ID // fallback for legacy handles without SpiritSessionID
	}
	suggType := "orchestration_optimization"
	title := fmt.Sprintf("编排优化建议: %s", biz.TruncateRunes(handle.ID, biz.MaxSuggestionTitleLen))

	// Dedup: skip if a pending suggestion with same type+title already exists
	pending, listErr := o.evolutionSugg.ListByAgent(ctx, targetID, "pending")
	if listErr != nil {
		o.lg.Warn("进化建议: 查询已有建议失败，跳过去重检查",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("target_id", targetID),
			loggateway.Err(listErr),
		)
		// Continue to create — better to risk a duplicate than to miss a suggestion
	} else {
		for _, s := range pending {
			if strings.EqualFold(strings.TrimSpace(s.Type), suggType) && strings.TrimSpace(s.Title) == title {
				o.lg.Info("进化建议: 已存在相同待处理建议，跳过创建",
					loggateway.StepID(biz.SpiritStepOrchestratorLearn),
					loggateway.Str("target_id", targetID),
					loggateway.Str("existing_id", s.ID),
				)
				return
			}
		}
	}

	content := fmt.Sprintf("编排 %q 的 DQ Score 为 %.2f（低于阈值 %.1f），当前拓扑 %s 执行效果不佳。", handle.ID, dqScore, biz.DQEvolutionThreshold, topology)
	if o.orchCache != nil {
		altTopology, altFound := o.orchCache.SuggestBestAlternativeTopology(handle.ID, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
	} else {
		content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
	}

	sugg, suggErr := o.evolutionSugg.Create(ctx, biz.EvolutionSuggestion{
		ID:        fmt.Sprintf("evo-orch-%s", uuid.NewString()[:12]),
		AgentID:   targetID,
		Type:      suggType,
		Title:     title,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if suggErr != nil {
		o.lg.Warn("创建编排优化建议失败",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(suggErr),
		)
		return
	}
	// Emit orchestration evolution suggested event as a v2 SystemNoticeEvent
	// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
	if o.eventBus != nil {
		meta := map[string]any{
			"event_type":        "orchestration.evolution_suggested",
			"orchestration_id":  handle.ID,
			"spirit_session_id": handle.SpiritSessionID,
			"dq_score":          dqScore,
			"topology":          string(topology),
			"suggestion_id":     sugg.ID,
		}
		o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(handle.SpiritSessionID, "orchestration.evolution_suggested", content, meta))
	}
}

// computeDQScoreFromSynthesis computes a DQ score from the synthesis output.
// - Successful synthesis with key findings: DQ = 0.8 + (findings_count * 0.05), capped at 1.0
// - Partial results: DQ = 0.5
// - Failed: DQ = 0.2
func computeDQScoreFromSynthesis(synthesis *biz.SynthesisOutput) float64 {
	if synthesis == nil {
		return 0.2
	}

	// Check if synthesis has meaningful content
	if synthesis.Content == "" && len(synthesis.TeamResults) == 0 {
		return 0.2
	}

	// Count completed team results
	completedCount := 0
	for _, tr := range synthesis.TeamResults {
		if tr.Status == "completed" {
			completedCount++
		}
	}

	// All teams failed
	if completedCount == 0 && len(synthesis.TeamResults) > 0 {
		return 0.2
	}

	// Partial success (some teams completed, some didn't)
	if completedCount > 0 && completedCount < len(synthesis.TeamResults) {
		return 0.5
	}

	// Full success — compute DQ from key findings
	dq := 0.8
	findingsCount := 0
	for _, tr := range synthesis.TeamResults {
		if tr.KeyFindings != "" {
			findingsCount++
		}
	}
	dq += float64(findingsCount) * 0.05
	if dq > 1.0 {
		dq = 1.0
	}
	return dq
}

// extractAgentKeysFromHandle extracts agent keys from the orchestration handle.
// It prefers handle.AgentKeys (real agent identifiers from AllocationPlan) over
// handle.TeamIDs (team identifiers) because TeamIDs and AgentKeys are different
// semantic entities — using TeamIDs as AgentKeys corrupts performance data.
func extractAgentKeysFromHandle(handle *biz.OrchestrationHandle) []string {
	// Prefer real agent keys from the allocation plan.
	if len(handle.AgentKeys) > 0 {
		return handle.AgentKeys
	}
	// Fallback: use TeamIDs as proxy for backward compatibility.
	var keys []string
	for _, teamID := range handle.TeamIDs {
		keys = append(keys, teamID)
	}
	if len(keys) == 0 {
		keys = append(keys, "spirit")
	}
	return keys
}

// saveInitialCheckpoint saves the first checkpoint for an orchestration lineage.
// This provides a baseline for recovery if the orchestration is interrupted.
func (o *TaskOrchestratorImpl) saveInitialCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o.checkpointSaver == nil {
		return
	}

	lineageID := handle.ID
	channelValues := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          string(handle.Strategy),
		"status":            string(handle.Status),
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	metadata := graph.NewCheckpointMetadata(graph.CheckpointSourceInput, -1)

	req := graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(lineageID, "", ""),
		Checkpoint: ckpt,
		Metadata:   metadata,
	}
	updatedConfig, err := o.checkpointSaver.Put(ctx, req)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to save initial checkpoint",
			loggateway.StepID(biz.SpiritStepOrchestratorCheckpoint),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(err),
		)
		return
	}

	handle.CheckpointID = graph.GetCheckpointID(updatedConfig)
	o.lg.Info("TaskOrchestrator: initial checkpoint saved",
		loggateway.StepID(biz.SpiritStepOrchestratorCheckpoint),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("checkpoint_id", handle.CheckpointID),
	)
}

// saveStepCheckpoint saves a checkpoint after a significant orchestration step.
func (o *TaskOrchestratorImpl) saveStepCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle, stepName string) {
	if o.checkpointSaver == nil {
		return
	}

	lineageID := handle.ID
	channelValues := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": handle.SpiritSessionID,
		"strategy":          string(handle.Strategy),
		"status":            string(handle.Status),
		"step":              stepName,
	}
	ckpt := graph.NewCheckpoint(channelValues, nil, nil)
	if handle.CheckpointID != "" {
		ckpt.ParentCheckpointID = handle.CheckpointID
	}
	metadata := graph.NewCheckpointMetadata(graph.CheckpointSourceLoop, 0)
	metadata.Extra["step"] = stepName

	req := graph.PutRequest{
		Config:     graph.CreateCheckpointConfig(lineageID, "", ""),
		Checkpoint: ckpt,
		Metadata:   metadata,
	}
	updatedConfig, err := o.checkpointSaver.Put(ctx, req)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to save step checkpoint",
			loggateway.StepID(biz.SpiritStepOrchestratorCheckpoint),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("step", stepName),
			loggateway.Err(err),
		)
		return
	}

	handle.CheckpointID = graph.GetCheckpointID(updatedConfig)
	o.lg.Info("TaskOrchestrator: step checkpoint saved",
		loggateway.StepID(biz.SpiritStepOrchestratorCheckpoint),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("step", stepName),
		loggateway.Str("checkpoint_id", handle.CheckpointID),
	)

	// Publish spirit_orchestration_checkpoint event.
	o.publishOrchestrationCheckpoint(ctx, handle, stepName)
}

// persistHandle persists the handle to the repository.
// Returns the persistence error so callers can decide how to handle it.
// On error paths (orchestrateXxx failed), callers typically log the error
// but do not propagate it, since the primary error is already being returned.
func (o *TaskOrchestratorImpl) persistHandle(ctx context.Context, handle *biz.OrchestrationHandle) error {
	if _, err := o.repo.Create(ctx, handle); err != nil {
		o.lg.Warn("TaskOrchestrator: failed to persist handle",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(err),
		)
		return err
	}
	return nil
}

// transitionOrchestrationStatus validates and applies a state transition on
// the orchestration handle using the AS-FSM-01 state machine.
//
// On illegal transitions, the target status is still applied (to preserve the
// existing behavior of recording the intended terminal state for debugging),
// but a warning is logged so operators can detect state-machine violations.
// This avoids breaking the orchestration flow when a transition rule is
// missing from the table, while still surfacing the violation.
func (o *TaskOrchestratorImpl) transitionOrchestrationStatus(ctx context.Context, handle *biz.OrchestrationHandle, target biz.OrchestrationStatus) {
	from := handle.Status
	if from == target {
		return
	}
	if !biz.CanTransitionOrchestrationStatus(from, target) {
		o.lg.Warn("TaskOrchestrator: illegal orchestration state transition",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("from", string(from)),
			loggateway.Str("to", string(target)),
		)
	}
	handle.Status = target
}

// marshalSynthesisOutput serializes a SynthesisOutput to JSON.
func marshalSynthesisOutput(output *biz.SynthesisOutput) (string, error) {
	type jsonOutput struct {
		Content       string                    `json:"content"`
		Strategy      string                    `json:"strategy"`
		TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
		SynthesizedAt string                    `json:"synthesized_at"`
	}
	jo := jsonOutput{
		Content:       output.Content,
		Strategy:      string(output.Strategy),
		TeamResults:   output.TeamResults,
		SynthesizedAt: output.SynthesizedAt,
	}
	b, err := json.Marshal(jo)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// publishOrchestrationStarted publishes the spirit_orchestration_started event as a v2 SystemNoticeEvent.
func (o *TaskOrchestratorImpl) publishOrchestrationStarted(ctx context.Context, handle *biz.OrchestrationHandle, taskPlan *biz.TaskPlan) {
	if o.eventBus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	meta := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"strategy":          string(handle.Strategy),
		"status":            string(handle.Status),
		"task_plan_id":      handle.TaskPlanID,
		"allocation_id":     handle.AllocationID,
	}
	if len(handle.TeamIDs) > 0 {
		meta["team_ids"] = handle.TeamIDs
	}
	// Attach parallel config so the frontend knows the team quota.
	pCfg := o.spiritUC.GetParallelConfig(ctx, spiritSessionID)
	if pCfg.MaxConcurrentTeams > 0 {
		meta["max_concurrent_teams"] = pCfg.MaxConcurrentTeams
	}
	meta["notice_type"] = "info"
	o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_started", "任务编排已启动", meta))
}

// publishOrchestrationCheckpoint publishes the spirit_orchestration_checkpoint event as a v2 SystemNoticeEvent.
func (o *TaskOrchestratorImpl) publishOrchestrationCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle, stepName string) {
	if o.eventBus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	status := biz.ActivityStatusRunning
	if handle.Status == biz.OrchestrationStatusCompleted {
		status = biz.ActivityStatusCompleted
	}
	meta := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"checkpoint_id":     handle.CheckpointID,
		"step":              stepName,
		"status":            string(handle.Status),
		"notice_type":       "info",
		"activity_status":   string(status),
	}
	o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_checkpoint", "编排检查点："+stepName, meta))
}

// publishOrchestrationInterrupted publishes the spirit_orchestration_interrupted event as a v2 SystemNoticeEvent.
func (o *TaskOrchestratorImpl) publishOrchestrationInterrupted(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o.eventBus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	meta := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"status":            string(handle.Status),
		"notice_type":       "warning",
	}
	o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_interrupted", "任务编排已中断", meta))
}

// sortByPerformance reorders agent keys by their historical performance for the
// given task type. Agents with performance data are sorted by success rate
// (descending), then by average DQ score (descending) as a tiebreaker.
// Agents without performance data retain their original relative order and
// appear after agents with data. If no performance data exists at all, the
// original order is preserved unchanged.
func (o *TaskOrchestratorImpl) sortByPerformance(ctx context.Context, agentKeys []string, taskType string) []string {
	if o.perfRepo == nil || len(agentKeys) <= 1 {
		return agentKeys
	}

	bestPerfs, err := o.perfRepo.GetBestForTaskType(ctx, taskType, len(agentKeys))
	if err != nil || len(bestPerfs) == 0 {
		o.lg.Debug("无 Agent 性能数据，保持原始排序",
			loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
			loggateway.Str("task_type", taskType),
		)
		return agentKeys
	}

	// Build a map: agentKey → (successRate, avgDQScore)
	type perfRank struct {
		successRate float64
		avgDQScore  float64
	}
	perfMap := make(map[string]perfRank, len(bestPerfs))
	for _, p := range bestPerfs {
		perfMap[p.AgentKey] = perfRank{
			successRate: p.SuccessRate,
			avgDQScore:  p.AvgDQScore,
		}
	}

	// Separate into two groups: with-perf and without-perf.
	var withPerf, withoutPerf []string
	for _, key := range agentKeys {
		if _, ok := perfMap[key]; ok {
			withPerf = append(withPerf, key)
		} else {
			withoutPerf = append(withoutPerf, key)
		}
	}

	// Sort withPerf group by success rate desc, then avgDQScore desc.
	sort.Slice(withPerf, func(i, j int) bool {
		pi := perfMap[withPerf[i]]
		pj := perfMap[withPerf[j]]
		if pi.successRate != pj.successRate {
			return pi.successRate > pj.successRate
		}
		return pi.avgDQScore > pj.avgDQScore
	})

	result := make([]string, 0, len(agentKeys))
	result = append(result, withPerf...)
	result = append(result, withoutPerf...)

	o.lg.Info("Agent 性能排序完成",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("task_type", taskType),
		loggateway.Int("with_perf_count", len(withPerf)),
		loggateway.Int("without_perf_count", len(withoutPerf)),
	)

	return result
}
