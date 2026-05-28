package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func (uc *GraphUsecase) notifyExecComplete(exec *GraphExecution) {
	if uc == nil || uc.execObserver == nil || exec == nil {
		return
	}
	uc.execObserver.OnGraphExecutionComplete(exec)
}

// evictIfNeeded removes the oldest finished execution if the executions map exceeds maxExecutions.
func (uc *GraphUsecase) evictIfNeeded() {
	if len(uc.executions) < maxExecutions {
		return
	}
	var oldestID string
	var oldestTime time.Time
	for id, exec := range uc.executions {
		if exec.Status == "running" || exec.Status == "waiting_human" {
			continue
		}
		finishTime := exec.StartedAt
		if exec.FinishedAt != nil {
			finishTime = *exec.FinishedAt
		}
		if oldestID == "" || finishTime.Before(oldestTime) {
			oldestID = id
			oldestTime = finishTime
		}
	}
	if oldestID != "" {
		delete(uc.executions, oldestID)
		delete(uc.teamBuildConfigs, oldestID)
	}
}

func (uc *GraphUsecase) loadExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if ok {
		return exec, nil
	}
	persisted, err := uc.runRepo.GetRun(ctx, executionID)
	if err != nil {
		return nil, ErrNotFound
	}
	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[executionID] = persisted
	uc.mu.Unlock()
	return persisted, nil
}

func (uc *GraphUsecase) buildConfigForExecution(ctx context.Context, exec *GraphExecution) (GraphBuildConfig, error) {
	if exec != nil {
		if cfg, ok := uc.teamBuildConfig(exec.ID); ok {
			return cfg, nil
		}
	}
	def, err := uc.GetGraph(ctx, exec.GraphID)
	if err != nil {
		return GraphBuildConfig{}, err
	}
	return FinalizeGraphFailurePolicy(defToBuildConfig(def)), nil
}

func (uc *GraphUsecase) ensureCheckpointRuntime(ctx context.Context, exec *GraphExecution) error {
	if exec.runtime != nil {
		return nil
	}
	if exec.GraphID == "" || exec.LineageID == "" {
		return ErrNotFound
	}
	cfg, err := uc.buildConfigForExecution(ctx, exec)
	if err != nil {
		return err
	}
	rt, err := uc.factory.BuildRuntime(ctx, cfg, exec.SessionID, exec.GraphID, exec.ID, exec.LineageID)
	if err != nil {
		return err
	}
	exec.runtime = rt
	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[exec.ID] = exec
	uc.mu.Unlock()
	return nil
}

func (uc *GraphUsecase) ExecuteGraph(ctx context.Context, graphID, sessionID, execID string, initialState map[string]any) (*GraphExecution, error) {
	if execID == "" {
		execID = uuid.New().String()
	}
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		uc.notifyExecComplete(&GraphExecution{ID: execID, GraphID: graphID, SessionID: sessionID, Status: "failed", ErrorMessage: err.Error()})
		return nil, err
	}

	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def))

	runtime, eventCh, err := uc.factory.BuildAndRun(ctx, cfg, sessionID, graphID, execID, initialState)
	if err != nil {
		uc.notifyExecComplete(&GraphExecution{ID: execID, GraphID: graphID, SessionID: sessionID, Status: "failed", ErrorMessage: err.Error()})
		return nil, err
	}

	exec := &GraphExecution{
		ID:        execID,
		GraphID:   graphID,
		SessionID: sessionID,
		Status:    "running",
		runtime:   runtime,
		LineageID: runtime.GetLineageID(),
		StartedAt: time.Now(),
	}

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		exec.Status = "failed"
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		return nil, kerrors.FromError(ErrGraphSaveRun).WithCause(err)
	}

	safego.Go(context.Background(), "graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return exec, nil
}

func (uc *GraphUsecase) ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID, execID string, cfg GraphBuildConfig, initialState map[string]any) (*GraphExecution, error) {
	if execID == "" {
		execID = uuid.New().String()
	}
	cfg = FinalizeGraphFailurePolicy(cfg)
	graphID = strings.TrimSpace(graphID)
	if graphID == "" {
		graphID = "compiled-graph"
	}

	runtime, eventCh, err := uc.factory.BuildAndRun(ctx, cfg, sessionID, graphID, execID, initialState)
	if err != nil {
		uc.notifyExecComplete(&GraphExecution{ID: execID, GraphID: graphID, SessionID: sessionID, Status: "failed", ErrorMessage: err.Error()})
		return nil, err
	}

	exec := &GraphExecution{
		ID:        execID,
		GraphID:   graphID,
		SessionID: sessionID,
		Status:    "running",
		runtime:   runtime,
		LineageID: runtime.GetLineageID(),
		StartedAt: time.Now(),
	}

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		exec.Status = "failed"
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		return nil, kerrors.FromError(ErrGraphSaveRun).WithCause(err)
	}

	safego.Go(context.Background(), "graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return exec, nil
}

func (uc *GraphUsecase) GetExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	return uc.loadExecution(ctx, executionID)
}

func (uc *GraphUsecase) ListExecutions(ctx context.Context, graphID string, pageSize int, pageToken string) ([]*GraphExecution, string, error) {
	return uc.runRepo.ListRunsByGraph(ctx, graphID, pageSize, pageToken)
}

func (uc *GraphUsecase) CancelExecution(ctx context.Context, executionID string) error {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if exec.Status != "running" && exec.Status != "waiting_human" {
		return ErrGraphInvalidStatus
	}
	if exec.runtime != nil {
		_ = exec.runtime.Cancel()
	}
	exec.Status = "cancelled"
	now := time.Now()
	exec.FinishedAt = &now
	return uc.runRepo.UpdateRun(ctx, exec)
}

func (uc *GraphUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if exec.Status != "waiting_human" && exec.Status != "running" {
		return nil, ErrGraphInvalidStatus
	}

	lineageID := exec.LineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		exec.LineageID = lineageID
	}

	if exec.runtime != nil {
		_ = exec.runtime.Cancel()
	}

	cfg, err := uc.buildConfigForExecution(ctx, exec)
	if err != nil {
		return nil, err
	}

	runtime, eventCh, err := uc.factory.BuildAndResume(ctx, cfg, exec.SessionID, exec.GraphID, executionID, lineageID, resumeValue)
	if err != nil {
		return nil, kerrors.FromError(ErrGraphResume).WithCause(err)
	}

	exec.runtime = runtime
	exec.Status = "running"
	exec.InterruptNode = ""

	safego.Go(context.Background(), "graph.consumeEvents(resume)", func() {
		uc.consumeRuntimeEvents(eventCh, exec, executionID, exec.GraphID, exec.SessionID, func() { uc.notifyExecComplete(exec) })
	})

	if err := uc.runRepo.UpdateRun(ctx, exec); err != nil {
		return nil, kerrors.InternalServer("GRAPH", "update run after resume").WithCause(err)
	}
	return exec, nil
}

func (uc *GraphUsecase) consumeRuntimeEvents(eventCh <-chan GraphRuntimeEvent, exec *GraphExecution, execID, graphID, sessionID string, onComplete func()) {
	for e := range eventCh {
		uc.updateExecutionFromRuntimeEvent(exec, e)
	}

	uc.mu.Lock()
	if exec.Status == "running" {
		exec.Status = "completed"
		now := time.Now()
		exec.FinishedAt = &now
	}
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
		event.SysLogWarn("system.tool.record_fail", "consumeRuntimeEvents: UpdateRun failed", event.P("exec_id", execID), event.P("error", err.Error()))
	}

	if onComplete != nil {
		onComplete()
	}
}

func (uc *GraphUsecase) updateExecutionFromRuntimeEvent(exec *GraphExecution, e GraphRuntimeEvent) {
	switch e.Type {
	case DomainEventGraphNodeStart:
		uc.mu.Lock()
		if exec.Status != "failed" {
			exec.CurrentNode = e.NodeID
		}
		uc.mu.Unlock()
		if uc.taskCoord != nil {
			ctx := context.Background()
			cfg, err := uc.buildConfigForExecution(ctx, exec)
			if err == nil {
				node := nodeDefFromConfig(cfg, e.NodeID)
				if ShouldCreateTaskForNode(node) {
					if err := uc.taskCoord.OnGraphNodeStart(ctx, exec, node, ""); err != nil {
						event.SysLogWarn("system.graph.task_start_fail", "graph task on node start failed",
							event.P("execution_id", exec.ID), event.P("node_id", e.NodeID), event.P("error", err.Error()))
					}
				}
			}
		}
	case DomainEventGraphNodeEnd:
		uc.mu.Lock()
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "completed",
			Timestamp: time.Now(),
		})
		uc.mu.Unlock()
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			event.SysLogWarn("system.tool.record_fail", "updateExecutionFromRuntimeEvent: UpdateRun failed for node_end", event.P("execution_id", exec.ID), event.P("error", err.Error()))
		}
	case DomainEventGraphNodeError:
		uc.mu.Lock()
		exec.ErrorMessage = e.Error
		exec.Status = "failed"
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "failed",
			Error:     e.Error,
			Timestamp: time.Now(),
		})
		uc.mu.Unlock()
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			event.SysLogWarn("system.tool.record_fail", "updateExecutionFromRuntimeEvent: UpdateRun failed for node_error", event.P("execution_id", exec.ID), event.P("error", err.Error()))
		}
	case DomainEventGraphInterrupt:
		uc.mu.Lock()
		exec.Status = "waiting_human"
		exec.InterruptNode = e.NodeID
		uc.mu.Unlock()
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			event.SysLogWarn("system.tool.record_fail", "updateExecutionFromRuntimeEvent: UpdateRun failed for interrupt", event.P("execution_id", exec.ID), event.P("error", err.Error()))
		}
	}
}

func upsertGraphStep(steps []GraphStepSnapshot, step GraphStepSnapshot) []GraphStepSnapshot {
	for i := range steps {
		if steps[i].NodeID == step.NodeID && steps[i].StepIndex == step.StepIndex {
			steps[i] = step
			return steps
		}
	}
	return append(steps, step)
}

func (uc *GraphUsecase) executionWithRuntime(ctx context.Context, executionID string) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if err := uc.ensureCheckpointRuntime(ctx, exec); err != nil {
		return nil, err
	}
	return exec, nil
}

func (uc *GraphUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (any, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelGetState(ctx, exec.LineageID, checkpointID, namespace)
}

func (uc *GraphUsecase) TimeTravelHistory(ctx context.Context, executionID string, namespace string, limit int) (any, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelHistory(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) TimeTravelEditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (any, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelEditState(ctx, exec.LineageID, checkpointID, namespace, patch)
}

func (uc *GraphUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) (any, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.ListCheckpoints(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) GetStateSnapshot(ctx context.Context, executionID string, checkpointID string, namespace string) (any, error) {
	return uc.TimeTravelGetState(ctx, executionID, checkpointID, namespace)
}

func (uc *GraphUsecase) EditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (any, error) {
	return uc.TimeTravelEditState(ctx, executionID, checkpointID, namespace, patch)
}
