package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
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

func (uc *GraphUsecase) buildConfigForExecution(ctx context.Context, exec *GraphExecution) (*CompiledTeam, error) {
	if exec != nil {
		if ct, ok := uc.teamBuildConfig(exec.ID); ok {
			return ct, nil
		}
	}
	if uc.compiledTeamRepo != nil && exec != nil && strings.HasPrefix(exec.GraphID, "team:") {
		parts := strings.SplitN(exec.GraphID, ":", 2)
		if len(parts) == 2 {
			ct, err := uc.compiledTeamRepo.Load(ctx, parts[1], exec.GraphID)
			if err == nil && ct != nil {
				uc.mu.Lock()
				if uc.teamBuildConfigs == nil {
					uc.teamBuildConfigs = make(map[string]*CompiledTeam)
				}
				uc.teamBuildConfigs[exec.ID] = ct
				uc.mu.Unlock()
				return ct, nil
			}
		}
	}
	def, err := uc.GetGraph(ctx, exec.GraphID)
	if err != nil {
		return nil, err
	}
	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)
	return NewCompiledTeam(cfg, nil, nil), nil
}

func (uc *GraphUsecase) ensureCheckpointRuntime(ctx context.Context, exec *GraphExecution) error {
	if exec.runtime != nil {
		return nil
	}
	if exec.GraphID == "" || exec.LineageID == "" {
		return ErrNotFound
	}
	ct, err := uc.buildConfigForExecution(ctx, exec)
	if err != nil {
		return err
	}
	rt, err := uc.factory.BuildRuntime(ctx, ct.GraphBuildConfig, exec.SessionID, exec.GraphID, exec.ID, exec.LineageID)
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

	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)

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
	cfg = FinalizeGraphFailurePolicy(cfg, nil, nil)
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

func (uc *GraphUsecase) ListExecutions(ctx context.Context, graphID string, pageSize int, pageToken string, opts ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return uc.runRepo.ListRunsByGraph(ctx, graphID, pageSize, pageToken, opts...)
}

func (uc *GraphUsecase) CancelExecution(ctx context.Context, executionID string) error {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return err
	}
	exec.execMu.Lock()
	if exec.Status != "running" && exec.Status != "waiting_human" {
		exec.execMu.Unlock()
		return ErrGraphInvalidStatus
	}
	if exec.runtime != nil {
		if err := exec.runtime.Cancel(); err != nil {
			uc.lg.Warn("cancel graph runtime", loggateway.Err(err))
		}
	}
	exec.Status = "cancelled"
	now := time.Now()
	exec.FinishedAt = &now
	exec.execMu.Unlock()
	return uc.runRepo.UpdateRun(ctx, exec)
}

func (uc *GraphUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	exec.execMu.Lock()
	if exec.Status != "waiting_human" && exec.Status != "running" {
		exec.execMu.Unlock()
		return nil, ErrGraphInvalidStatus
	}

	lineageID := exec.LineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		exec.LineageID = lineageID
	}

	if exec.runtime != nil {
		if err := exec.runtime.Cancel(); err != nil {
			uc.lg.Warn("cancel graph runtime on resume", loggateway.Err(err))
		}
	}
	exec.execMu.Unlock()

	ct, err := uc.buildConfigForExecution(ctx, exec)
	if err != nil {
		return nil, err
	}

	runtime, eventCh, err := uc.factory.BuildAndResume(ctx, ct.GraphBuildConfig, exec.SessionID, exec.GraphID, executionID, lineageID, resumeValue)
	if err != nil {
		return nil, kerrors.FromError(ErrGraphResume).WithCause(err)
	}

	exec.execMu.Lock()
	exec.runtime = runtime
	exec.Status = "running"
	exec.execMu.Unlock()
	exec.interruptMu.Lock()
	exec.interrupted = false
	exec.InterruptNode = ""
	exec.interruptMu.Unlock()

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

	exec.execMu.Lock()
	if exec.Status == "running" {
		exec.Status = "completed"
		now := time.Now()
		exec.FinishedAt = &now
	}
	wasEvicted := exec.evicted
	exec.execMu.Unlock()

	if !wasEvicted {
		uc.mu.Lock()
		uc.evictIfNeeded()
		uc.executions[execID] = exec
		uc.mu.Unlock()
	}
	if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
		uc.lg.Warn("consumeRuntimeEvents: UpdateRun failed", loggateway.StepID("graph.record_fail"), loggateway.Str("exec_id", execID), loggateway.Err(err))
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
			ct, err := uc.buildConfigForExecution(ctx, exec)
			if err == nil {
				node := nodeDefFromConfig(ct.GraphBuildConfig, e.NodeID)
				var meta NodeTaskMeta
				if m, ok := ct.TaskMeta[e.NodeID]; ok {
					meta = m
				}
				if ShouldCreateTaskForNode(node, meta) {
					if err := uc.taskCoord.OnGraphNodeStart(ctx, exec, node, meta, ""); err != nil {
						uc.lg.Warn("graph task on node start failed",
						loggateway.StepID("graph.task_start_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Str("node_id", e.NodeID), loggateway.Err(err))
					}
				}
			}
		}
	case DomainEventGraphNodeEnd:
		var stepsSnapshot []GraphStepSnapshot
		uc.mu.Lock()
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "completed",
			Timestamp: time.Now(),
		})
		stepsSnapshot = make([]GraphStepSnapshot, len(exec.Steps))
		copy(stepsSnapshot, exec.Steps)
		uc.mu.Unlock()
		exec.Steps = stepsSnapshot
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_end", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphNodeError:
		var stepsSnapshot []GraphStepSnapshot
		var statusSnapshot string
		var errMsgSnapshot string
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
		stepsSnapshot = make([]GraphStepSnapshot, len(exec.Steps))
		copy(stepsSnapshot, exec.Steps)
		statusSnapshot = exec.Status
		errMsgSnapshot = exec.ErrorMessage
		uc.mu.Unlock()
		exec.Steps = stepsSnapshot
		exec.Status = statusSnapshot
		exec.ErrorMessage = errMsgSnapshot
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_error", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphInterrupt:
		var statusSnapshot string
		var interruptNodeSnapshot string
		exec.interruptMu.Lock()
		exec.interrupted = true
		exec.InterruptNode = e.NodeID
		interruptNodeSnapshot = exec.InterruptNode
		exec.interruptMu.Unlock()
		uc.mu.Lock()
		exec.Status = "waiting_human"
		statusSnapshot = exec.Status
		uc.mu.Unlock()
		exec.Status = statusSnapshot
		exec.InterruptNode = interruptNodeSnapshot
		if err := uc.runRepo.UpdateRun(context.Background(), exec); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for interrupt", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
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
