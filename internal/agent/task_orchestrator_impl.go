package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
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
	spiritUC    *biz.SpiritTeamUsecase
	assembler   SpiritTeamAssemblerPort
	compiler    *DAGToGraphCompiler
	repo        biz.OrchestrationRepository
	matcher     biz.AgentMatcherPort
	deps        TRPCBuilderDeps
	synthesis   SpiritSynthesisPort
	lg          loggateway.Logger
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
	lg loggateway.Logger,
) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		spiritUC:  spiritUC,
		assembler: assembler,
		compiler:  compiler,
		repo:      repo,
		matcher:   matcher,
		deps:      deps,
		synthesis: synthesis,
		lg:        lg,
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
	return nil
}

// Synthesize synthesizes the results of the orchestration.
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
		// Persist synthesis result to handle.
		if output != nil {
			synthesisJSON, marshalErr := marshalSynthesisOutput(output)
			if marshalErr == nil {
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
		}
		return output, nil
	}

	return nil, kerrors.InternalServer("SPIRIT", "synthesis service not available")
}

// Recover recovers an interrupted orchestration from its last checkpoint.
// Stub implementation - full recovery logic will be implemented in T2.6.
func (o *TaskOrchestratorImpl) Recover(ctx context.Context, orchestrationID string) error {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return kerrors.NotFound("SPIRIT", "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusInterrupted {
		return kerrors.BadRequest("SPIRIT", "only interrupted orchestrations can be recovered")
	}

	o.lg.Info("TaskOrchestrator: recovery stub - marking as running",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
		loggateway.Str("checkpoint_id", handle.CheckpointID),
	)

	// TODO(T2.6): Load Checkpoint, rebuild GraphAgent, resume execution.
	handle.Status = biz.OrchestrationStatusRunning
	_, err = o.repo.Update(ctx, handle)
	return err
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
		Content       string                   `json:"content"`
		Strategy      string                   `json:"strategy"`
		TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
		SynthesizedAt string                   `json:"synthesized_at"`
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
