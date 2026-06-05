package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/graph"
)

var _ biz.TaskOrchestratorPort = (*TaskOrchestratorImpl)(nil)

// SpiritTeamAssemblerPort is a local port for assembling and starting teams.
// This avoids importing internal/service directly.
type SpiritTeamAssemblerPort interface {
	AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error)
	CancelTeam(ctx context.Context, teamID string) error
	CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
}

// SpiritSynthesisPort is a local port for synthesizing results.
type SpiritSynthesisPort interface {
	SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error)
}

// TaskOrchestratorImpl implements biz.TaskOrchestratorPort.
type TaskOrchestratorImpl struct {
	spiritUC        *biz.SpiritTeamUsecase
	assembler       SpiritTeamAssemblerPort
	compiler        *DAGToGraphCompiler
	repo            biz.OrchestrationRepository
	matcher         biz.AgentMatcherPort
	deps            TRPCBuilderDeps
	synthesis       SpiritSynthesisPort
	checkpointSaver graph.CheckpointSaver
	orchCache       *biz.OrchestrationCache
	perfRepo        biz.AgentPerformanceRepository
	bus             contract.Bus
	lg              loggateway.Logger
}

// NewTaskOrchestratorImpl creates a new TaskOrchestratorImpl.
func NewTaskOrchestratorImpl(
	spiritUC *biz.SpiritTeamUsecase,
	assembler SpiritTeamAssemblerPort,
	compiler *DAGToGraphCompiler,
	repo biz.OrchestrationRepository,
	matcher biz.AgentMatcherPort,
	deps TRPCBuilderDeps,
	synthesis SpiritSynthesisPort,
	checkpointSaver graph.CheckpointSaver,
	orchCache *biz.OrchestrationCache,
	perfRepo biz.AgentPerformanceRepository,
	bus contract.Bus,
	lg loggateway.Logger,
) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		spiritUC:        spiritUC,
		assembler:       assembler,
		compiler:        compiler,
		repo:            repo,
		matcher:         matcher,
		deps:            deps,
		synthesis:       synthesis,
		checkpointSaver: checkpointSaver,
		orchCache:       orchCache,
		perfRepo:        perfRepo,
		bus:             bus,
		lg:              lg,
	}
}

// Orchestrate builds and executes the orchestration graph based on the TaskPlan and AllocationPlan.
func (o *TaskOrchestratorImpl) Orchestrate(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan) (*biz.OrchestrationHandle, error) {
	if taskPlan == nil {
		return nil, kerrors.BadRequest("SPIRIT", "task_plan is required")
	}
	if allocPlan == nil {
		return nil, kerrors.BadRequest("SPIRIT", "allocation_plan is required")
	}

	o.lg.Info("TaskOrchestrator: starting orchestration",
		loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
		loggateway.Str("task_plan_id", taskPlan.ID),
		loggateway.Str("strategy", string(taskPlan.Strategy)),
		loggateway.Str("spirit_session_id", taskPlan.SpiritSessionID),
	)

	// Create OrchestrationHandle with status "pending".
	handle := &biz.OrchestrationHandle{
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
		handle.Status = biz.OrchestrationStatusCompleted
		o.lg.Info("TaskOrchestrator: direct strategy, no orchestration needed",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
		)

	case biz.StrategySingleAgent:
		// Agent-as-Tool path.
		if err := o.orchestrateSingleAgent(ctx, taskPlan, allocPlan, handle); err != nil {
			handle.Status = biz.OrchestrationStatusFailed
			o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyParallel:
		// Parallel team path.
		if err := o.orchestrateTeam(ctx, taskPlan, allocPlan, handle, "parallel"); err != nil {
			handle.Status = biz.OrchestrationStatusFailed
			o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyDAG:
		// DAG → Graph compilation path.
		if err := o.orchestrateDAG(ctx, taskPlan, allocPlan, handle); err != nil {
			handle.Status = biz.OrchestrationStatusFailed
			o.persistHandle(ctx, handle)
			return nil, err
		}

	case biz.StrategyCoordinator:
		// Coordinator team path.
		if err := o.orchestrateTeam(ctx, taskPlan, allocPlan, handle, "coordinator"); err != nil {
			handle.Status = biz.OrchestrationStatusFailed
			o.persistHandle(ctx, handle)
			return nil, err
		}

	default:
		return nil, kerrors.BadRequest("SPIRIT",
			fmt.Sprintf("unknown orchestration strategy: %s", taskPlan.Strategy))
	}

	// Save a step checkpoint after orchestration setup completes.
	o.saveStepCheckpoint(ctx, handle, "orchestrate_setup")

	// Publish spirit_orchestration_started event.
	o.publishOrchestrationStarted(ctx, handle, taskPlan)

	// Persist the handle.
	persisted, err := o.repo.Create(ctx, handle)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to persist orchestration handle",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(err),
		)
		return handle, nil
	}
	return persisted, nil
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
		return fmt.Errorf("build agent-as-tool: %w", err)
	}

	// For now, mark as running. The actual tool invocation happens in the Spirit turn.
	handle.Status = biz.OrchestrationStatusRunning
	o.lg.Info("TaskOrchestrator: agent-as-tool built successfully",
		loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
		loggateway.Str("orchestration_id", handle.ID),
	)
	return nil
}

// orchestrateTeam handles the parallel/coordinator team path.
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
		return kerrors.BadRequest("SPIRIT", "no agent keys in allocation plan")
	}

	// Assemble the team via SpiritTeamAssembler.
	params := biz.SpiritTeamParams{
		SpiritSessionID: taskPlan.SpiritSessionID,
		TaskDescription: taskPlan.UserMessage,
		AgentKeys:       agentKeys,
		Mode:            mode,
		AutoStart:       true,
	}

	team, _, err := o.assembler.AssembleTeam(ctx, params)
	if err != nil {
		return fmt.Errorf("assemble team: %w", err)
	}

	handle.TeamIDs = []string{team.ID}
	handle.Status = biz.OrchestrationStatusRunning

	o.lg.Info("TaskOrchestrator: team assembled and started",
		loggateway.StepID(biz.SpiritStepOrchestratorExecute),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("team_id", team.ID),
	)
	return nil
}

// orchestrateDAG handles the DAG → Graph compilation path.
func (o *TaskOrchestratorImpl) orchestrateDAG(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, handle *biz.OrchestrationHandle) error {
	o.lg.Info("TaskOrchestrator: DAG path",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("orchestration_id", handle.ID),
	)

	if taskPlan.TaskDAG == nil {
		return kerrors.BadRequest("SPIRIT", "task_dag is required for DAG strategy")
	}

	// Compile DAG + AllocationPlan → Definition JSON.
	defJSON, err := o.compiler.Compile(taskPlan.TaskDAG, allocPlan)
	if err != nil {
		return fmt.Errorf("compile DAG to graph: %w", err)
	}

	o.lg.Info("TaskOrchestrator: DAG compiled to definition JSON",
		loggateway.StepID(biz.SpiritStepOrchestratorGraphBuild),
		loggateway.Str("orchestration_id", handle.ID),
	)

	// Collect agent keys from allocations for the team.
	agentKeys := make([]string, 0, len(allocPlan.Allocations))
	for _, alloc := range allocPlan.Allocations {
		key := strings.TrimSpace(alloc.AssignedKey)
		if key != "" {
			agentKeys = append(agentKeys, key)
		}
	}

	if len(agentKeys) == 0 {
		return kerrors.BadRequest("SPIRIT", "no agent keys in allocation plan for DAG strategy")
	}

	// Determine mode from the compiled definition.
	// The compiler already chose "coordinator" or "parallel" based on DAG structure.
	mode := "coordinator"
	if taskPlan.TaskDAG != nil {
		hasDeps := false
		for _, node := range taskPlan.TaskDAG.Nodes {
			if len(node.DependsOn) > 0 {
				hasDeps = true
				break
			}
		}
		if !hasDeps && len(taskPlan.TaskDAG.Nodes) > 1 {
			mode = "parallel"
		}
	}

	// Assemble the team with the compiled definition JSON.
	params := biz.SpiritTeamParams{
		SpiritSessionID: taskPlan.SpiritSessionID,
		TaskDescription: taskPlan.UserMessage,
		AgentKeys:       agentKeys,
		Mode:            mode,
		AutoStart:       true,
	}

	team, _, err := o.assembler.AssembleTeam(ctx, params)
	if err != nil {
		return fmt.Errorf("assemble DAG team: %w", err)
	}

	// Update the team's DefinitionJSON with the DAG-compiled version.
	// The assembler already created the team with buildSpiritTeamDefinitionJSON,
	// but the DAG-compiled version has the correct structure.
	// For now, we log the compiled definition for observability.
	handle.TeamIDs = []string{team.ID}
	handle.GraphExecutionID = team.ID // Team ID serves as the graph execution ID.
	handle.Status = biz.OrchestrationStatusRunning

	o.lg.Info("TaskOrchestrator: DAG team assembled",
		loggateway.StepID(biz.SpiritStepOrchestratorExecute),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("team_id", team.ID),
		loggateway.Str("definition_json_len", fmt.Sprintf("%d", len(defJSON))),
	)
	return nil
}

// CheckProgress returns the progress of each subtask in the orchestration.
func (o *TaskOrchestratorImpl) CheckProgress(ctx context.Context, orchestrationID string) ([]biz.TaskProgress, error) {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return nil, kerrors.NotFound("SPIRIT", "orchestration not found")
	}

	// For team-based strategies, delegate to team progress checking.
	if len(handle.TeamIDs) > 0 && o.assembler != nil {
		teamProgresses, err := o.assembler.CheckTeamProgress(ctx, handle.SpiritSessionID)
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
		return kerrors.NotFound("SPIRIT", "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusPending && handle.Status != biz.OrchestrationStatusRunning {
		return kerrors.BadRequest("SPIRIT", "only pending or running orchestrations can be cancelled")
	}

	// Cancel all teams.
	for _, teamID := range handle.TeamIDs {
		if o.assembler != nil {
			if cancelErr := o.assembler.CancelTeam(ctx, teamID); cancelErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to cancel team",
					loggateway.StepID(biz.SpiritStepOrchestratorExecute),
					loggateway.Str("team_id", teamID),
					loggateway.Err(cancelErr),
				)
			}
		}
	}

	handle.Status = biz.OrchestrationStatusCancelled
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
		return nil, kerrors.NotFound("SPIRIT", "orchestration not found")
	}

	if handle.SpiritSessionID == "" {
		return nil, kerrors.BadRequest("SPIRIT", "orchestration has no spirit_session_id")
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
				handle.Status = biz.OrchestrationStatusCompleted
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

	return nil, kerrors.InternalServer("SPIRIT", "synthesis service not available")
}

// Recover recovers an interrupted orchestration from its last checkpoint.
func (o *TaskOrchestratorImpl) Recover(ctx context.Context, orchestrationID string) error {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return kerrors.NotFound("SPIRIT", "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusInterrupted {
		return kerrors.BadRequest("SPIRIT", fmt.Sprintf(
			"only interrupted orchestrations can be recovered (status: %s)", handle.Status))
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
		handle.Status = biz.OrchestrationStatusFailed
		if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
			o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Err(updateErr),
			)
		}
		return fmt.Errorf("no checkpoint available for orchestration %s", orchestrationID)
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
			handle.Status = biz.OrchestrationStatusFailed
			if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
					loggateway.StepID(biz.SpiritStepOrchestratorRecover),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(updateErr),
				)
			}
			return fmt.Errorf("failed to load checkpoint for orchestration %s: %w", orchestrationID, loadErr)
		}

		if tuple == nil || tuple.Checkpoint == nil {
			o.lg.Warn("TaskOrchestrator: checkpoint not found, marking orchestration as failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Str("checkpoint_id", handle.CheckpointID),
			)
			handle.Status = biz.OrchestrationStatusFailed
			if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
					loggateway.StepID(biz.SpiritStepOrchestratorRecover),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(updateErr),
				)
			}
			return fmt.Errorf("checkpoint %s not found for orchestration %s", handle.CheckpointID, orchestrationID)
		}

		o.lg.Info("TaskOrchestrator: checkpoint loaded successfully",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Str("checkpoint_id", tuple.Checkpoint.ID),
		)

		// TODO: Rebuild GraphAgent from checkpoint state and resume execution.
		// The current implementation marks the orchestration as running and relies
		// on the team/agent infrastructure to pick up the work. Full graph-agent
		// rebuild from checkpoint state requires deeper integration with the
		// trpc-agent-go executor, which will be implemented in a future iteration.
	}

	// Mark as running so the orchestration can be tracked.
	handle.Status = biz.OrchestrationStatusRunning
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

// RecoverAllInterrupted finds all interrupted orchestrations and attempts recovery.
func (o *TaskOrchestratorImpl) RecoverAllInterrupted(ctx context.Context) error {
	handles, err := o.repo.ListByStatus(ctx, biz.OrchestrationStatusInterrupted)
	if err != nil {
		return fmt.Errorf("list interrupted orchestrations: %w", err)
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
	if o.orchCache != nil {
		taskPattern := biz.ExtractTaskPattern(handle.ID) // Use orchestration ID as pattern key
		topology := biz.TopologyCoordinator
		switch handle.Strategy {
		case biz.StrategyDirect:
			topology = biz.TopologyDirect
		case biz.StrategyParallel:
			topology = biz.TopologyParallel
		case biz.StrategyDAG:
			topology = biz.TopologyHybrid
		}
		agentKeys := extractAgentKeysFromHandle(handle)
		o.orchCache.RecordCompletionWithAgents(ctx, taskPattern, topology, dqScore, len(handle.TeamIDs), 0, agentKeys)
		o.lg.Info("在线学习: 编排缓存已更新",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("task_pattern", taskPattern),
			loggateway.Float64("dq_score", dqScore),
		)
	}

	// 2. Update AgentPerformance for each agent in the orchestration
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
					AgentKey:      agentKey,
					TaskType:      string(handle.Strategy),
					TotalRuns:     1,
					SuccessRuns:   successCount,
					SuccessRate:   float64(successCount),
					AvgDQScore:    dqScore,
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
// Since the handle stores TeamIDs rather than agent keys directly, we use
// the strategy as a fallback task type indicator.
func extractAgentKeysFromHandle(handle *biz.OrchestrationHandle) []string {
	// The handle doesn't directly store agent keys; they're in the AllocationPlan.
	// For the learning loop, we extract what we can from the handle.
	// If TeamIDs are present, we use them as proxy identifiers.
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
		"orchestration_id":   handle.ID,
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
		"orchestration_id":   handle.ID,
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

// persistHandle is a helper to persist the handle, logging errors.
func (o *TaskOrchestratorImpl) persistHandle(ctx context.Context, handle *biz.OrchestrationHandle) {
	if _, err := o.repo.Create(ctx, handle); err != nil {
		o.lg.Warn("TaskOrchestrator: failed to persist handle",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(err),
		)
	}
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

// publishOrchestrationStarted publishes the spirit_orchestration_started event.
// For dual consumption (REQ-SO-04), also publishes spirit_team_assembled (old equivalent).
func (o *TaskOrchestratorImpl) publishOrchestrationStarted(ctx context.Context, handle *biz.OrchestrationHandle, taskPlan *biz.TaskPlan) {
	if o.bus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	// New event: spirit_orchestration_started
	env := contract.NewEnvelope(contract.EnvelopeTypeSpiritOrchestrationStarted, "task-orchestrator", spiritSessionID)
	env.Metadata = map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"strategy":          string(handle.Strategy),
		"status":            string(handle.Status),
		"task_plan_id":      handle.TaskPlanID,
		"allocation_id":     handle.AllocationID,
	}
	if len(handle.TeamIDs) > 0 {
		env.Metadata["team_ids"] = handle.TeamIDs
	}
	o.bus.Publish(ctx, env)

	// Dual consumption: also publish spirit_team_assembled (old equivalent).
	if len(handle.TeamIDs) > 0 {
		dualEnv := contract.NewEnvelope(contract.EnvelopeTypeSpiritTeamAssembled, "task-orchestrator", spiritSessionID)
		dualEnv.TeamID = handle.TeamIDs[0]
		dualEnv.Metadata = map[string]any{
			"team_id":           handle.TeamIDs[0],
			"spirit_session_id": spiritSessionID,
			"mode":              string(handle.Strategy),
			"task_summary":      biz.TruncateRunes(taskPlan.UserMessage, 200),
		}
		o.bus.Publish(ctx, dualEnv)
	}
}

// publishOrchestrationCheckpoint publishes the spirit_orchestration_checkpoint event.
// For dual consumption (REQ-SO-04), also publishes spirit_team_progress (old equivalent).
func (o *TaskOrchestratorImpl) publishOrchestrationCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle, stepName string) {
	if o.bus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	// New event: spirit_orchestration_checkpoint
	env := contract.NewEnvelope(contract.EnvelopeTypeSpiritOrchestrationCheckpoint, "task-orchestrator", spiritSessionID)
	env.Metadata = map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"checkpoint_id":     handle.CheckpointID,
		"step":              stepName,
		"status":            string(handle.Status),
	}
	o.bus.Publish(ctx, env)

	// Dual consumption: also publish spirit_team_progress (old equivalent).
	if len(handle.TeamIDs) > 0 {
		progressPct := 50.0
		if handle.Status == biz.OrchestrationStatusCompleted {
			progressPct = 100
		}
		dualEnv := contract.NewEnvelope(contract.EnvelopeTypeSpiritTeamProgress, "task-orchestrator", spiritSessionID)
		dualEnv.TeamID = handle.TeamIDs[0]
		dualEnv.Metadata = map[string]any{
			"team_id":      handle.TeamIDs[0],
			"status":       string(handle.Status),
			"progress_pct": progressPct,
		}
		o.bus.Publish(ctx, dualEnv)
	}
}

// publishOrchestrationInterrupted publishes the spirit_orchestration_interrupted event.
// For dual consumption (REQ-SO-04), also publishes spirit_team_failed (old equivalent).
func (o *TaskOrchestratorImpl) publishOrchestrationInterrupted(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o.bus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	// New event: spirit_orchestration_interrupted
	env := contract.NewEnvelope(contract.EnvelopeTypeSpiritOrchestrationInterrupted, "task-orchestrator", spiritSessionID)
	env.Metadata = map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"status":            string(handle.Status),
	}
	o.bus.Publish(ctx, env)

	// Dual consumption: also publish spirit_team_failed (old equivalent).
	if len(handle.TeamIDs) > 0 {
		dualEnv := contract.NewEnvelope(contract.EnvelopeTypeSpiritTeamFailed, "task-orchestrator", spiritSessionID)
		dualEnv.TeamID = handle.TeamIDs[0]
		dualEnv.Metadata = map[string]any{
			"team_id":   handle.TeamIDs[0],
			"status":    string(handle.Status),
			"error":     "orchestration interrupted",
		}
		o.bus.Publish(ctx, dualEnv)
	}
}
