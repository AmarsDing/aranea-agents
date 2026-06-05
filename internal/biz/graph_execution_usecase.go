package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// GraphExecutionUsecase handles graph execution lifecycle: run, resume, cancel,
// checkpoint/time-travel, and in-memory execution cache with GC.
type GraphExecutionUsecase struct {
	cacheMgr     *GraphCacheManager
	runRepo      GraphRunRepo
	factory      GraphBuilderFactory
	execObserver GraphExecutionObserver
	taskCoord    GraphTaskCoordinator
	defProvider  GraphDefinitionProvider
	mu           sync.RWMutex
	executions   map[string]*GraphExecution
	lg           loggateway.Logger
}

// NewGraphExecutionUsecase creates an execution usecase with in-memory execution cache.
func NewGraphExecutionUsecase(
	runRepo GraphRunRepo,
	factory GraphBuilderFactory,
	observer GraphExecutionObserver,
	cacheMgr *GraphCacheManager,
	defProvider GraphDefinitionProvider,
	lg loggateway.Logger,
) *GraphExecutionUsecase {
	uc := &GraphExecutionUsecase{
		runRepo:      runRepo,
		factory:      factory,
		execObserver: observer,
		cacheMgr:     cacheMgr,
		defProvider:  defProvider,
		executions:   make(map[string]*GraphExecution),
		lg:           lg,
	}
	safego.Go(appctx.Ctx(), "graph-gc-loop", func() { uc.gcLoop() })
	return uc
}

// SetTaskCoordinator sets the task coordinator for graph-node-to-task-board wiring.
func (uc *GraphExecutionUsecase) SetTaskCoordinator(c GraphTaskCoordinator) {
	uc.taskCoord = c
}

// CacheMgr returns the embedded GraphCacheManager for callers that need cache operations.
func (uc *GraphExecutionUsecase) CacheMgr() *GraphCacheManager { return uc.cacheMgr }

// ---------------------------------------------------------------------------
// Execution CRUD
// ---------------------------------------------------------------------------

func (uc *GraphExecutionUsecase) notifyExecComplete(exec *GraphExecution) {
	if uc == nil || uc.execObserver == nil || exec == nil {
		return
	}
	uc.execObserver.OnGraphExecutionComplete(exec)
}

// evictIfNeeded removes the oldest finished execution if the executions map exceeds maxExecutions.
// Caller must hold uc.mu (write lock). This method reads exec fields under execMu protection.
func (uc *GraphExecutionUsecase) evictIfNeeded() {
	if len(uc.executions) < maxExecutions {
		return
	}
	var oldestID string
	var oldestTime time.Time
	for id, exec := range uc.executions {
		exec.execMu.RLock()
		status := exec.Status
		finishTime := exec.StartedAt
		if exec.FinishedAt != nil {
			finishTime = *exec.FinishedAt
		}
		exec.execMu.RUnlock()
		if status == "running" || status == "waiting_human" {
			continue
		}
		if oldestID == "" || finishTime.Before(oldestTime) {
			oldestID = id
			oldestTime = finishTime
		}
	}
	if oldestID != "" {
		if exec, ok := uc.executions[oldestID]; ok {
			exec.execMu.RLock()
			rt := exec.runtime
			exec.execMu.RUnlock()
			if rt != nil {
				if err := rt.Cancel(); err != nil {
					uc.lg.Warn("cancel graph runtime on evict", loggateway.Err(err))
				}
			}
			exec.SetEvicted()
		}
		delete(uc.executions, oldestID)
		uc.cacheMgr.RemoveBuildConfig(oldestID)
	}
}

func (uc *GraphExecutionUsecase) loadExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
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

func (uc *GraphExecutionUsecase) ensureCheckpointRuntime(ctx context.Context, exec *GraphExecution) error {
	if exec.runtime != nil {
		return nil
	}
	if exec.GraphID == "" || exec.LineageID == "" {
		return ErrNotFound
	}
	ct, err := uc.cacheMgr.BuildConfigForExecution(ctx, exec)
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

// ExecuteGraph runs a stored graph by ID with the given initial state.
func (uc *GraphExecutionUsecase) ExecuteGraph(ctx context.Context, graphID, sessionID, execID string, initialState map[string]any) (*GraphExecution, error) {
	if execID == "" {
		execID = uuid.New().String()
	}
	def, err := uc.defProvider.GetGraph(ctx, graphID)
	if err != nil {
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, "failed")
	failedExec.ErrorMessage = err.Error()
	uc.notifyExecComplete(failedExec)
		return nil, err
	}

	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)

	runtime, eventCh, err := uc.factory.BuildAndRun(ctx, cfg, sessionID, graphID, execID, initialState)
	if err != nil {
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, "failed")
	failedExec.ErrorMessage = err.Error()
	uc.notifyExecComplete(failedExec)
		return nil, err
	}

	exec := NewGraphExecution(context.WithoutCancel(ctx), execID, graphID, sessionID, "running")
	exec.runtime = runtime
	exec.LineageID = runtime.GetLineageID()

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		exec.Status = "failed"
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		return nil, kerrors.FromError(ErrGraphSaveRun).WithCause(err)
	}

	safego.Go(appctx.Ctx(), "graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return exec, nil
}

// ExecuteGraphBuildConfig runs a graph from a build config with the given initial state.
func (uc *GraphExecutionUsecase) ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID, execID string, cfg GraphBuildConfig, initialState map[string]any) (*GraphExecution, error) {
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
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, "failed")
		failedExec.ErrorMessage = err.Error()
		uc.notifyExecComplete(failedExec)
		return nil, err
	}

	exec := NewGraphExecution(context.WithoutCancel(ctx), execID, graphID, sessionID, "running")
	exec.runtime = runtime
	exec.LineageID = runtime.GetLineageID()

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		exec.Status = "failed"
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		return nil, kerrors.FromError(ErrGraphSaveRun).WithCause(err)
	}

	safego.Go(appctx.Ctx(), "graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return exec, nil
}

// GetExecution retrieves an execution by ID (from cache or persistence).
func (uc *GraphExecutionUsecase) GetExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	return uc.loadExecution(ctx, executionID)
}

// ListExecutions lists executions for a graph with pagination.
func (uc *GraphExecutionUsecase) ListExecutions(ctx context.Context, graphID string, pageSize int, pageToken string, opts ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return uc.runRepo.ListRunsByGraph(ctx, graphID, pageSize, pageToken, opts...)
}

// CancelExecution cancels a running or waiting execution.
func (uc *GraphExecutionUsecase) CancelExecution(ctx context.Context, executionID string) error {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return err
	}
	var persistSnap *GraphExecution
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
	persistSnap = exec.SnapshotForPersist()
	exec.execMu.Unlock()
	return uc.runRepo.UpdateRun(ctx, persistSnap)
}

// ResumeExecution resumes an interrupted execution with the given value.
func (uc *GraphExecutionUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
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

	ct, err := uc.cacheMgr.BuildConfigForExecution(ctx, exec)
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
	exec.ctx = context.WithoutCancel(ctx)
	exec.execMu.Unlock()
	exec.interruptMu.Lock()
	exec.interrupted = false
	exec.InterruptNode = ""
	exec.interruptMu.Unlock()

	safego.Go(appctx.Ctx(), "graph.consumeEvents(resume)", func() {
		uc.consumeRuntimeEvents(eventCh, exec, executionID, exec.GraphID, exec.SessionID, func() { uc.notifyExecComplete(exec) })
	})

	var persistSnap *GraphExecution
	exec.execMu.Lock()
	persistSnap = exec.SnapshotForPersist()
	exec.execMu.Unlock()
	if err := uc.runRepo.UpdateRun(ctx, persistSnap); err != nil {
		return nil, kerrors.InternalServer("GRAPH", "update run after resume").WithCause(err)
	}
	return exec, nil
}

// ---------------------------------------------------------------------------
// Team graph execution
// ---------------------------------------------------------------------------

// RegisterTeamGraphExecution indexes a team GraphAgent run for task/resume coordination (M53 Phase 7).
// Build config is kept in-memory; graph_id uses the team: prefix (not a persisted graph asset).
func (uc *GraphExecutionUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *CompiledTeam) error {
	if uc == nil {
		return nil
	}
	execID = strings.TrimSpace(execID)
	if execID == "" {
		return kerrors.BadRequest("GRAPH", "graph execution id required")
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return kerrors.BadRequest("GRAPH", "team id required")
	}
	graphID := GraphIDTeamPrefix + teamID
	if teamRunID != "" {
		graphID = graphID + ":" + strings.TrimSpace(teamRunID)
	}
	exec := NewGraphExecution(context.Background(), execID, graphID, strings.TrimSpace(sessionID), TeamRunStatusRunning)

	if uc.runRepo != nil {
		if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
			return err
		}
	}

	uc.cacheMgr.SaveCompiledTeam(ctx, teamID, graphID, strings.TrimSpace(sessionID), ct)
	uc.cacheMgr.SetTeamBuildConfig(execID, ct)

	uc.mu.Lock()
	uc.evictIfNeeded()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return nil
}

// MarkTeamGraphInterrupt records HITL/checkpoint pause for a team graph execution.
func (uc *GraphExecutionUsecase) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	if uc == nil {
		return nil
	}
	exec, err := uc.loadExecution(ctx, strings.TrimSpace(execID))
	if err != nil {
		return err
	}
	nodeID = strings.TrimSpace(nodeID)
	lineageID = strings.TrimSpace(lineageID)
	exec.interruptMu.Lock()
	exec.interrupted = true
	exec.InterruptNode = nodeID
	exec.interruptMu.Unlock()
	var persistSnap *GraphExecution
	exec.execMu.Lock()
	exec.Status = TeamRunStatusWaitingHuman
	exec.CurrentNode = nodeID
	if lineageID != "" {
		exec.LineageID = lineageID
	}
	persistSnap = exec.SnapshotForPersist()
	exec.execMu.Unlock()
	if uc.runRepo == nil {
		return nil
	}
	return uc.runRepo.UpdateRun(ctx, persistSnap)
}

// ---------------------------------------------------------------------------
// Runtime event processing
// ---------------------------------------------------------------------------

func (uc *GraphExecutionUsecase) consumeRuntimeEvents(eventCh <-chan GraphRuntimeEvent, exec *GraphExecution, execID, graphID, sessionID string, onComplete func()) {
	for e := range eventCh {
		uc.updateExecutionFromRuntimeEvent(exec, e)
	}

	var persistSnap *GraphExecution
	exec.execMu.Lock()
	if exec.Status == "running" {
		exec.Status = "completed"
		now := time.Now()
		exec.FinishedAt = &now
	}
	persistSnap = exec.SnapshotForPersist()
	wasEvicted := exec.evicted
	exec.execMu.Unlock()

	if !wasEvicted {
		uc.mu.Lock()
		uc.evictIfNeeded()
		uc.executions[execID] = exec
		uc.mu.Unlock()
	}
	if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
		uc.lg.Warn("consumeRuntimeEvents: UpdateRun failed", loggateway.StepID("graph.record_fail"), loggateway.Str("exec_id", execID), loggateway.Err(err))
	}

	if onComplete != nil {
		onComplete()
	}
}

func (uc *GraphExecutionUsecase) updateExecutionFromRuntimeEvent(exec *GraphExecution, e GraphRuntimeEvent) {
	switch e.Type {
	case DomainEventGraphNodeStart:
		exec.execMu.Lock()
		if exec.Status != "failed" {
			exec.CurrentNode = e.NodeID
		}
		exec.execMu.Unlock()
		if uc.taskCoord != nil {
			ctx := exec.ctx
			ct, err := uc.cacheMgr.BuildConfigForExecution(ctx, exec)
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
		var persistSnap *GraphExecution
		exec.execMu.Lock()
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "completed",
			Timestamp: time.Now(),
		})
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_end", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphNodeError:
		var persistSnap *GraphExecution
		exec.execMu.Lock()
		exec.ErrorMessage = e.Error
		exec.Status = "failed"
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "failed",
			Error:     e.Error,
			Timestamp: time.Now(),
		})
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_error", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphInterrupt:
		var persistSnap *GraphExecution
		exec.interruptMu.Lock()
		exec.interrupted = true
		exec.InterruptNode = e.NodeID
		exec.interruptMu.Unlock()
		exec.execMu.Lock()
		exec.Status = "waiting_human"
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for interrupt", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	}
}

// ---------------------------------------------------------------------------
// Checkpoint / Time-travel
// ---------------------------------------------------------------------------

func (uc *GraphExecutionUsecase) executionWithRuntime(ctx context.Context, executionID string) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if err := uc.ensureCheckpointRuntime(ctx, exec); err != nil {
		return nil, err
	}
	return exec, nil
}

// TimeTravelGetState retrieves a state snapshot at a specific checkpoint.
func (uc *GraphExecutionUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (*GraphCheckpointState, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelGetState(ctx, exec.LineageID, checkpointID, namespace)
}

// TimeTravelHistory lists checkpoint history for an execution.
func (uc *GraphExecutionUsecase) TimeTravelHistory(ctx context.Context, executionID string, namespace string, limit int) (GraphCheckpointList, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelHistory(ctx, exec.LineageID, namespace, limit)
}

// TimeTravelEditState edits state at a specific checkpoint.
func (uc *GraphExecutionUsecase) TimeTravelEditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (*GraphEditedState, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.TimeTravelEditState(ctx, exec.LineageID, checkpointID, namespace, patch)
}

// ListCheckpoints lists checkpoints for an execution.
func (uc *GraphExecutionUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) (GraphCheckpointList, error) {
	exec, err := uc.executionWithRuntime(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return exec.runtime.ListCheckpoints(ctx, exec.LineageID, namespace, limit)
}

// GetStateSnapshot retrieves a state snapshot at a checkpoint.
func (uc *GraphExecutionUsecase) GetStateSnapshot(ctx context.Context, executionID string, checkpointID string, namespace string) (*GraphCheckpointState, error) {
	return uc.TimeTravelGetState(ctx, executionID, checkpointID, namespace)
}

// EditState edits state at a checkpoint.
func (uc *GraphExecutionUsecase) EditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (*GraphEditedState, error) {
	return uc.TimeTravelEditState(ctx, executionID, checkpointID, namespace, patch)
}

// ---------------------------------------------------------------------------
// GC
// ---------------------------------------------------------------------------

func (uc *GraphExecutionUsecase) gcLoop() {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for range ticker.C {
		uc.gc()
	}
}

func (uc *GraphExecutionUsecase) gc() {
	uc.mu.Lock()
	var expired []*GraphExecution
	now := time.Now()
	for id, exec := range uc.executions {
		exec.execMu.RLock()
		status := exec.Status
		startedAt := exec.StartedAt
		var finishedAt *time.Time
		if exec.FinishedAt != nil {
			ft := *exec.FinishedAt
			finishedAt = &ft
		}
		rt := exec.runtime
		exec.execMu.RUnlock()

		if status == "running" || status == "waiting_human" {
			continue
		}
		if finishedAt != nil && now.Sub(*finishedAt) > executionMaxAge {
			exec.SetEvicted()
			delete(uc.executions, id)
			uc.cacheMgr.RemoveBuildConfig(id)
		} else if finishedAt == nil && now.Sub(startedAt) > executionMaxAge {
			if rt != nil {
				if err := rt.Cancel(); err != nil {
					uc.lg.Warn("cancel graph runtime on gc eviction", loggateway.Err(err))
				}
			}
			exec.execMu.Lock()
			exec.Status = "failed"
			exec.ErrorMessage = "execution expired: no activity within timeout"
			nowCopy := now
			exec.FinishedAt = &nowCopy
			exec.execMu.Unlock()
			exec.SetEvicted()
			expired = append(expired, exec)
			delete(uc.executions, id)
			uc.cacheMgr.RemoveBuildConfig(id)
		}
	}
	uc.mu.Unlock()

	// Persist expired executions to repo before discarding from memory.
	for _, exec := range expired {
		persistCtx := exec.ctx
		if persistCtx == nil {
			persistCtx = context.Background()
		}
		if err := uc.runRepo.UpdateRun(persistCtx, exec); err != nil {
			uc.lg.Error("gc expired execution persist failed", loggateway.StepID("graph.gc_expired_persist"), loggateway.Str("run_id", exec.ID), loggateway.Err(err))
		}
	}
}
