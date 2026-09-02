package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// GraphExecutionUsecase handles graph execution lifecycle: run, resume, cancel,
// checkpoint/time-travel, and in-memory execution cache with GC.
// TECH-DEBT(COG): file_lines=827, limit=500 (AS-COG-01); split checkpoint/time-travel
// and GC into sub-files in a future iteration.
type GraphExecutionUsecase struct {
	cacheMgr     *GraphCacheManager
	runRepo      GraphRunRepo
	factory      GraphRunnerFactory
	execObserver GraphExecutionObserver
	taskCoord    GraphTaskCoordinator
	runEventSink GraphRunEventSink
	defProvider  GraphDefinitionProvider
	sm           *GraphExecutionStateMachine
	mu           sync.RWMutex
	executions   map[string]*GraphExecution
	gcConfig     GraphGCConfig
	gcCancel     chan struct{}
	lg           loggateway.Logger
}

// NewGraphExecutionUsecase creates an execution usecase with in-memory execution cache.
func NewGraphExecutionUsecase(
	runRepo GraphRunRepo,
	factory GraphRunnerFactory,
	observer GraphExecutionObserver,
	cacheMgr *GraphCacheManager,
	defProvider GraphDefinitionProvider,
	lg loggateway.Logger,
	gcConfig GraphGCConfig,
) *GraphExecutionUsecase {
	if gcConfig.Interval <= 0 || gcConfig.ExecutionMaxAge <= 0 || gcConfig.MaxExecutions <= 0 {
		gcConfig = DefaultGraphGCConfig()
	}
	uc := &GraphExecutionUsecase{
		runRepo:      runRepo,
		factory:      factory,
		execObserver: observer,
		cacheMgr:     cacheMgr,
		defProvider:  defProvider,
		sm:           NewGraphExecutionStateMachine(),
		executions:   make(map[string]*GraphExecution),
		gcConfig:     gcConfig,
		lg:           lg,
	}
	uc.gcCancel = make(chan struct{})
	safego.Go(appctx.Ctx(), "graph-gc-loop", func() { uc.gcLoop() })
	return uc
}

// applyExecTransition validates and applies a state transition via the state machine.
// Returns an error if the transition is illegal; in that case exec.Status is NOT modified.
// Caller must hold exec.execMu (write lock).
//
// For error-recovery paths where the caller has already validated the transition or
// where leaving the status unchanged is acceptable on illegal transition, the error
// can be ignored. Do NOT silently apply a target state on illegal transition —
// either fix the caller to not request illegal transitions, or extend the state
// machine rules to include the needed transition.
func (uc *GraphExecutionUsecase) applyExecTransition(exec *GraphExecution, event GraphExecutionEvent) error {
	from := ParseGraphExecutionState(exec.Status)
	newState, err := uc.sm.Transition(from, event)
	if err != nil {
		uc.lg.Warn("graph: illegal state transition rejected by FSM",
			loggateway.StepID("graph.fsm_rejected"),
			loggateway.Str("execution_id", exec.ID),
			loggateway.Str("from", string(from)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return apierror.BadRequest(apierror.DomainGraph, "invalid graph execution transition from %s via %s", from, event)
	}
	exec.Status = string(newState)
	return nil
}

// SetTaskCoordinator sets the task coordinator for graph-node-to-task-board wiring.
func (uc *GraphExecutionUsecase) SetTaskCoordinator(c GraphTaskCoordinator) {
	uc.taskCoord = c
}

// SetRunEventSink registers an external run-event bridge (e.g. the
// twinmonitor OpenAPI compat facade). Optional; nil disables callbacks.
// Set once during wire assembly before the server starts accepting traffic.
func (uc *GraphExecutionUsecase) SetRunEventSink(s GraphRunEventSink) {
	uc.runEventSink = s
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

// evictIfNeeded removes the oldest finished execution if the executions map exceeds MaxExecutions.
// Caller must hold uc.mu (write lock). This method reads exec fields under execMu protection.
func (uc *GraphExecutionUsecase) evictIfNeeded() {
	if len(uc.executions) < uc.gcConfig.MaxExecutions {
		return
	}
	var oldestID string
	var oldestTime time.Time
	for id, exec := range uc.executions {
		exec.execMu.RLock()
		status := ParseGraphExecutionState(exec.Status)
		finishTime := exec.StartedAt
		if exec.FinishedAt != nil {
			finishTime = *exec.FinishedAt
		}
		exec.execMu.RUnlock()
		if status.IsActive() {
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
	// S4（双收敛竞态）：GetRun 期间另一 goroutine 可能已完成加载并缓存。
	// 必须双重检查并共享同一实例——真实 DB repo 每次返回新对象，两个实例
	// 各有独立 execMu，终态幂等检查（FinalizeTeamGraphExecution）会失效，
	// 并发 finalize（success/failed）双写 persist 后写覆盖先写。
	if existing, ok := uc.executions[executionID]; ok {
		uc.mu.Unlock()
		return existing, nil
	}
	uc.executions[executionID] = persisted
	// Evict after inserting so the newly loaded execution is considered in
	// the eviction ranking. evictIfNeeded skips Running/WaitingHuman, so
	// active executions are safe. Terminal executions may be evicted, but
	// they can always be reloaded from the repo on next access.
	uc.evictIfNeeded()
	uc.mu.Unlock()
	return persisted, nil
}

// cacheNewExecution inserts a freshly created execution BEFORE SaveRun (S2
// registration race). With the old save-then-insert order, a concurrent
// loadExecution landing between SaveRun and the cache insert would cache a
// second DB-loaded instance; the registration insert then overwrote it,
// splitting the execution into two objects with independent execMu and
// breaking terminal-state idempotency (same root cause as S4). Insert-first
// makes the new instance canonical from birth. Returns false (without
// overwriting) when the ID is already cached — e.g. a duplicate registration,
// which the subsequent SaveRun will reject with a conflict.
func (uc *GraphExecutionUsecase) cacheNewExecution(exec *GraphExecution) bool {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if _, ok := uc.executions[exec.ID]; ok {
		return false
	}
	uc.evictIfNeeded()
	uc.executions[exec.ID] = exec
	return true
}

// uncacheExecution rolls back cacheNewExecution when the subsequent SaveRun
// fails. It only removes the entry if it is still the same instance.
func (uc *GraphExecutionUsecase) uncacheExecution(exec *GraphExecution) {
	uc.mu.Lock()
	if cur, ok := uc.executions[exec.ID]; ok && cur == exec {
		delete(uc.executions, exec.ID)
	}
	uc.mu.Unlock()
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
	rt, err := uc.factory.BuildRuntime(ctx, ct.GraphBuildConfig, exec.SessionID, exec.SpiritSessionID, exec.GraphID, exec.ID, exec.LineageID)
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
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, string(GraphExecFailed))
		failedExec.SpiritSessionID = sessionID
		failedExec.ErrorMessage = err.Error()
		uc.notifyExecComplete(failedExec)
		return nil, err
	}

	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)

	// 异步执行必须脱离请求生命周期：HTTP/gRPC handler 返回后请求 ctx 会被取消，
	// 若直接传给 BuildAndRun，executor 的事件流随请求结束被 cancel 提前关闭，
	// consumeRuntimeEvents 会把"零节点执行"误判为 completed（steps=null）。
	// 取消语义由 runtime 自带的 runCancel 承担（CancelExecution 路径）。
	runCtx := context.WithoutCancel(ctx)
	runtime, eventCh, err := uc.factory.BuildAndRun(runCtx, cfg, sessionID, sessionID, graphID, execID, initialState)
	if err != nil {
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, string(GraphExecFailed))
		failedExec.SpiritSessionID = sessionID
		failedExec.ErrorMessage = err.Error()
		uc.notifyExecComplete(failedExec)
		return nil, err
	}

	exec := NewGraphExecution(context.WithoutCancel(ctx), execID, graphID, sessionID, string(GraphExecRunning))
	exec.SpiritSessionID = sessionID
	exec.runtime = runtime
	exec.LineageID = runtime.GetLineageID()
	// Y4: record the definition identity this execution runs against; Resume
	// rejects when the graph was edited after the checkpoint was written.
	exec.DefinitionHash = ComputeGraphBuildConfigHash(cfg)

	// S2：insert-first，使 exec 从出生即 canonical（见 cacheNewExecution）。
	inserted := uc.cacheNewExecution(exec)
	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		if inserted {
			uc.uncacheExecution(exec)
		}
		if tErr := uc.applyExecTransition(exec, GraphExecEventFail); tErr != nil {
			uc.lg.Warn("applyExecTransition failed on error path",
				loggateway.Str("exec_id", execID),
				loggateway.Str("graph_id", graphID),
				loggateway.Err(tErr))
		}
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		e := apierror.Internal("GRAPH", "graph execute save run failed")
		e.Cause = err
		return nil, e
	}

	streamGen := exec.NextStreamGen()
	safego.GoBackground("graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, streamGen, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

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

	// 同 ExecuteGraph：脱离请求生命周期，避免 handler 返回后事件流被提前关闭。
	runCtx := context.WithoutCancel(ctx)
	runtime, eventCh, err := uc.factory.BuildAndRun(runCtx, cfg, sessionID, sessionID, graphID, execID, initialState)
	if err != nil {
		failedExec := NewGraphExecution(context.Background(), execID, graphID, sessionID, string(GraphExecFailed))
		failedExec.SpiritSessionID = sessionID
		failedExec.ErrorMessage = err.Error()
		uc.notifyExecComplete(failedExec)
		return nil, err
	}

	exec := NewGraphExecution(context.WithoutCancel(ctx), execID, graphID, sessionID, string(GraphExecRunning))
	exec.SpiritSessionID = sessionID
	exec.runtime = runtime
	exec.LineageID = runtime.GetLineageID()
	// Y4: record the definition identity this execution runs against; Resume
	// rejects when the graph was edited after the checkpoint was written.
	exec.DefinitionHash = ComputeGraphBuildConfigHash(cfg)

	// S2：insert-first，使 exec 从出生即 canonical（见 cacheNewExecution）。
	inserted := uc.cacheNewExecution(exec)
	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		if inserted {
			uc.uncacheExecution(exec)
		}
		if tErr := uc.applyExecTransition(exec, GraphExecEventFail); tErr != nil {
			uc.lg.Warn("applyExecTransition failed on error path",
				loggateway.Str("exec_id", execID),
				loggateway.Str("graph_id", graphID),
				loggateway.Err(tErr))
		}
		exec.ErrorMessage = err.Error()
		uc.notifyExecComplete(exec)
		e := apierror.Internal("GRAPH", "graph execute save run failed")
		e.Cause = err
		return nil, e
	}

	streamGen := exec.NextStreamGen()
	safego.GoBackground("graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, streamGen, execID, graphID, sessionID, func() { uc.notifyExecComplete(exec) })
	})

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
	if !ParseGraphExecutionState(exec.Status).IsActive() {
		exec.execMu.Unlock()
		return ErrGraphInvalidStatus
	}
	if exec.runtime != nil {
		if err := exec.runtime.Cancel(); err != nil {
			uc.lg.Warn("cancel graph runtime", loggateway.Err(err))
		}
	}
	uc.applyExecTransition(exec, GraphExecEventCancel)
	now := time.Now()
	exec.FinishedAt = &now
	persistSnap = exec.SnapshotForPersist()
	graphID := exec.GraphID
	exec.execMu.Unlock()
	if uc.runEventSink != nil {
		uc.runEventSink.OnRunCancelled(ctx, executionID, graphID)
	}
	return uc.runRepo.UpdateRun(ctx, persistSnap)
}

// ResumeExecution resumes an interrupted execution with the given value.
func (uc *GraphExecutionUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	exec.execMu.Lock()
	if _, err := uc.sm.Transition(ParseGraphExecutionState(exec.Status), GraphExecEventResume); err != nil {
		exec.execMu.Unlock()
		return nil, ErrGraphInvalidStatus
	}

	lineageID := exec.LineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		exec.LineageID = lineageID
	}

	// Cancel old runtime while still holding the lock to prevent concurrent
	// Resume from observing a non-terminal status and racing to build a new one.
	oldRuntime := exec.runtime
	// Transition status immediately to block concurrent Resume calls.
	// If BuildAndResume fails below, we roll back to WaitingHuman.
	uc.applyExecTransition(exec, GraphExecEventResume)
	exec.ctx = context.WithoutCancel(ctx)
	exec.execMu.Unlock()

	if oldRuntime != nil {
		if err := oldRuntime.Cancel(); err != nil {
			uc.lg.Warn("cancel graph runtime on resume", loggateway.Err(err))
		}
	}

	ct, err := uc.cacheMgr.BuildConfigForExecution(ctx, exec)
	if err != nil {
		// Roll back status on failure (running → waiting_human via interrupt semantics).
		exec.execMu.Lock()
		uc.applyExecTransition(exec, GraphExecEventInterrupt)
		exec.execMu.Unlock()
		return nil, err
	}

	// Y4: the graph definition must be identical to the one the execution
	// started with. After C1 the team graph is a real asset — editing it
	// re-materializes the definition, and resuming a stale checkpoint against
	// a changed topology routes checkpointed state through wrong nodes/edges.
	// Legacy rows (empty hash, pre-20261214) skip the check.
	exec.execMu.RLock()
	storedHash := exec.DefinitionHash
	exec.execMu.RUnlock()
	if storedHash != "" {
		if currentHash := ComputeGraphBuildConfigHash(ct.GraphBuildConfig); currentHash != storedHash {
			exec.execMu.Lock()
			uc.applyExecTransition(exec, GraphExecEventInterrupt)
			exec.execMu.Unlock()
			uc.lg.Warn("graph: resume rejected, definition changed since checkpoint",
				loggateway.StepID("graph.resume_hash_mismatch"),
				loggateway.Str("execution_id", executionID),
				loggateway.Str("graph_id", exec.GraphID))
			return nil, apierror.Conflict(apierror.DomainGraph, "graph definition changed since this execution paused; start a new execution instead of resuming")
		}
	}

	// 同 Run 路径（L261）：resume 的事件流也必须脱离请求生命周期——HTTP
	// handler 返回后请求 ctx 被取消，会把新 runtime 的事件流提前掐断
	// （executor EmitEventWithTimeout: context canceled），run 被误判为
	// failed（stream terminated without completion event）。取消语义由
	// runtime 自带 runCancel 承担（CancelExecution / 下次 Resume 路径）。
	runCtx := context.WithoutCancel(ctx)
	runtime, eventCh, err := uc.factory.BuildAndResume(runCtx, ct.GraphBuildConfig, exec.SessionID, exec.SpiritSessionID, exec.GraphID, executionID, lineageID, resumeValue)
	if err != nil {
		// Roll back status on failure (running → waiting_human via interrupt semantics).
		exec.execMu.Lock()
		uc.applyExecTransition(exec, GraphExecEventInterrupt)
		exec.execMu.Unlock()
		e := apierror.Internal("GRAPH", "graph resume failed")
		e.Cause = err
		return nil, e
	}

	exec.execMu.Lock()
	exec.runtime = runtime
	exec.execMu.Unlock()
	exec.interruptMu.Lock()
	exec.interrupted = false
	exec.InterruptNode = ""
	exec.interruptMu.Unlock()

	// Y2: bump the stream generation BEFORE spawning the new consumer. Any
	// still-draining stale consumer (old runtime cancel is async) observes the
	// generation mismatch and skips terminal convergence + status mutations.
	streamGen := exec.NextStreamGen()
	safego.GoBackground("graph.consumeEvents(resume)", func() {
		uc.consumeRuntimeEvents(eventCh, exec, streamGen, executionID, exec.GraphID, exec.SessionID, func() { uc.notifyExecComplete(exec) })
	})

	var persistSnap *GraphExecution
	exec.execMu.Lock()
	persistSnap = exec.SnapshotForPersist()
	exec.execMu.Unlock()
	if err := uc.runRepo.UpdateRun(ctx, persistSnap); err != nil {
		e := apierror.Internal("GRAPH", "update run after resume")
		e.Cause = err
		return nil, e
	}
	return exec, nil
}

// ---------------------------------------------------------------------------
// Team graph execution（崩溃恢复 RecoverOrphanedExecution 见 graph_execution_recover.go，83）
// ---------------------------------------------------------------------------

// RegisterTeamGraphExecution indexes a team GraphAgent run for task/resume coordination (M53 Phase 7).
// Build config is kept in-memory. graph_id 优先使用 team 的 linked_graph_id
// （C1 全量物化：真实图资产，team 多次 Run 共享同一资产 ID → /graphs/:id/executions
// 自然聚合执行历史）；linked 为空（存量未迁移）时保留 team: 合成 ID 兜底。
func (uc *GraphExecutionUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *CompiledTeam) error {
	if uc == nil {
		return nil
	}
	execID = strings.TrimSpace(execID)
	if execID == "" {
		return apierror.BadRequest("GRAPH", "graph execution id required")
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return apierror.BadRequest("GRAPH", "team id required")
	}
	graphID := strings.TrimSpace(linkedGraphID)
	if graphID == "" {
		graphID = GraphIDTeamPrefix + teamID
		if teamRunID != "" {
			graphID = graphID + ":" + strings.TrimSpace(teamRunID)
		}
	}
	exec := NewGraphExecution(context.Background(), execID, graphID, strings.TrimSpace(sessionID), string(GraphExecRunning))
	exec.SpiritSessionID = strings.TrimSpace(spiritSessionID)
	// Y4: record the definition identity for the resume consistency check.
	// The hash covers the build config that actually runs (ct.GraphBuildConfig),
	// identical in form to what BuildConfigForExecution resolves on resume.
	if ct != nil {
		exec.DefinitionHash = ComputeGraphBuildConfigHash(ct.GraphBuildConfig)
	}

	// S2：insert-first，使 exec 从出生即 canonical（见 cacheNewExecution）。
	inserted := uc.cacheNewExecution(exec)
	if uc.runRepo != nil {
		if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
			if inserted {
				uc.uncacheExecution(exec)
			}
			return err
		}
	}

	uc.cacheMgr.SaveCompiledTeam(ctx, teamID, graphID, strings.TrimSpace(sessionID), ct)
	uc.cacheMgr.SetTeamBuildConfig(execID, ct)
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
	var persistSnap *GraphExecution
	exec.execMu.Lock()
	if transErr := uc.applyExecTransition(exec, GraphExecEventInterrupt); transErr != nil {
		exec.execMu.Unlock()
		return transErr
	}
	exec.CurrentNode = nodeID
	if lineageID != "" {
		exec.LineageID = lineageID
	}
	exec.interruptMu.Lock()
	exec.interrupted = true
	exec.InterruptNode = nodeID
	exec.interruptMu.Unlock()
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

// consumeRuntimeEvents drains the runtime event stream and converges terminal
// state. gen is the stream generation captured at spawn time (Y2): when a
// newer stream (Resume) has taken over, this stale consumer skips terminal
// convergence, persistence, and completion callbacks.
func (uc *GraphExecutionUsecase) consumeRuntimeEvents(eventCh <-chan GraphRuntimeEvent, exec *GraphExecution, gen int64, execID, graphID, sessionID string, onComplete func()) {
	sawDone := false
	for e := range eventCh {
		if e.Type == DomainEventGraphDone {
			sawDone = true
		}
		uc.updateExecutionFromRuntimeEvent(exec, gen, e)
	}

	var persistSnap *GraphExecution
	var wasEvicted bool
	var persistCtx context.Context
	completed := false
	failMsg := ""
	exec.execMu.Lock()
	if exec.streamGen != gen {
		// Y2: a newer stream has taken over (Resume). This stale consumer must
		// not converge terminal state — otherwise the old stream ending without
		// a done event would falsely fail the new, still-running stream.
		exec.execMu.Unlock()
		uc.lg.Debug("graph: stale stream consumer exits without terminal convergence",
			loggateway.StepID("graph.stream_stale"),
			loggateway.Str("execution_id", execID),
			loggateway.Int64("gen", gen))
		return
	}
	// Terminal convergence is done-driven (N1): only an explicit framework
	// completion event may complete the execution. If the stream ends while
	// still running without a done event (fatal graph error whose Pregel
	// error event was the last on the wire, or premature termination),
	// fail-closed instead of reporting a false success.
	// Evicted executions (GC cancelled the runtime) close the channel
	// prematurely and must not be overridden either.
	if exec.Status == string(GraphExecRunning) && !exec.evicted {
		now := time.Now()
		if sawDone {
			uc.applyExecTransition(exec, GraphExecEventComplete)
			completed = true
		} else {
			exec.ErrorMessage = "graph execution stream terminated without completion event"
			uc.applyExecTransition(exec, GraphExecEventFail)
			failMsg = exec.ErrorMessage
		}
		exec.FinishedAt = &now
	}
	persistSnap = exec.SnapshotForPersist()
	wasEvicted = exec.evicted
	persistCtx = exec.ctx
	startedAt := exec.StartedAt
	exec.execMu.Unlock()

	if completed && uc.runEventSink != nil {
		if outSink, ok := uc.runEventSink.(GraphRunEventSinkOutput); ok {
			output, nodeOutputs := extractRunOutputs(persistSnap.CurrentState)
			outSink.OnRunOutput(persistCtx, execID, graphID, output, nodeOutputs)
		}
		uc.runEventSink.OnRunCompleted(persistCtx, execID, graphID, time.Since(startedAt).Milliseconds())
	}
	if failMsg != "" && uc.runEventSink != nil {
		uc.runEventSink.OnRunFailed(persistCtx, execID, graphID, failMsg)
	}

	if !wasEvicted {
		uc.mu.Lock()
		uc.evictIfNeeded()
		uc.executions[execID] = exec
		uc.mu.Unlock()
	}
	if persistCtx == nil {
		persistCtx = context.Background()
	}
	if err := uc.runRepo.UpdateRun(persistCtx, persistSnap); err != nil {
		uc.lg.Warn("consumeRuntimeEvents: UpdateRun failed", loggateway.StepID("graph.record_fail"), loggateway.Str("exec_id", execID), loggateway.Err(err))
	}

	if onComplete != nil {
		onComplete()
	}
}

// extractRunOutputs 从终态 state 提取运行输出与各节点输出。
// 键名对齐 trpc graph 框架：last_response / node_responses / output（图自定义字段）。
func extractRunOutputs(state map[string]any) (string, map[string]string) {
	if len(state) == 0 {
		return "", nil
	}
	var nodeOutputs map[string]string
	if nr, ok := state["node_responses"].(map[string]any); ok {
		nodeOutputs = make(map[string]string, len(nr))
		for nodeID, v := range nr {
			if s, ok := v.(string); ok && s != "" {
				nodeOutputs[nodeID] = s
			}
		}
	}
	output, _ := state["output"].(string)
	if output == "" {
		output, _ = state["last_response"].(string)
	}
	return output, nodeOutputs
}

// TECH-DEBT(COG): method_lines=91, limit=80 (AS-COG-01); the per-event-type
// switch should be extracted into small handlers in a future iteration.
func (uc *GraphExecutionUsecase) updateExecutionFromRuntimeEvent(exec *GraphExecution, gen int64, e GraphRuntimeEvent) {
	switch e.Type {
	case DomainEventGraphDone:
		// 终态 state 落库（current_state_json）：供节点输出提取与排查。
		if len(e.FinalState) > 0 {
			exec.execMu.Lock()
			exec.CurrentState = deepCopyMap(e.FinalState)
			exec.execMu.Unlock()
		}
	case DomainEventGraphNodeStart:
		exec.execMu.Lock()
		if ParseGraphExecutionState(exec.Status) != GraphExecFailed {
			exec.CurrentNode = e.NodeID
		}
		exec.execMu.Unlock()
		if uc.runEventSink != nil {
			uc.runEventSink.OnNodeStarted(exec.ctx, exec.ID, exec.GraphID, e.NodeID, e.StepNumber)
		}
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
		if uc.runEventSink != nil {
			uc.runEventSink.OnNodeCompleted(exec.ctx, exec.ID, exec.GraphID, e.NodeID, e.StepNumber, "completed", "")
		}
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_end", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphNodeError:
		var persistSnap *GraphExecution
		exec.execMu.Lock()
		if exec.streamGen != gen {
			// Y2: stale stream — must not fail the newer stream's execution.
			exec.execMu.Unlock()
			return
		}
		if !e.Retrying {
			// Final failure (retries exhausted or no retry policy): fail the execution.
			exec.ErrorMessage = e.Error
			uc.applyExecTransition(exec, GraphExecEventFail)
		}
		// Intermediate retry failures only record the step attempt snapshot;
		// the retry may still succeed and the graph may complete normally.
		exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
			NodeID:    e.NodeID,
			StepIndex: e.StepNumber,
			Status:    "failed",
			Error:     e.Error,
			Timestamp: time.Now(),
		})
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if uc.runEventSink != nil {
			uc.runEventSink.OnNodeCompleted(exec.ctx, exec.ID, exec.GraphID, e.NodeID, e.StepNumber, "failed", e.Error)
			if !e.Retrying {
				uc.runEventSink.OnRunFailed(exec.ctx, exec.ID, exec.GraphID, e.Error)
			}
		}
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for node_error", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphExecutionError:
		// N1: graph-level fatal (Pregel error: panic / max steps / executeGraph
		// failure). Fail the execution regardless of node-level progress.
		var persistSnap *GraphExecution
		exec.execMu.Lock()
		if exec.streamGen != gen {
			// Y2: stale stream — must not fail the newer stream's execution.
			exec.execMu.Unlock()
			return
		}
		if ParseGraphExecutionState(exec.Status) == GraphExecRunning {
			exec.ErrorMessage = e.Error
			uc.applyExecTransition(exec, GraphExecEventFail)
			now := time.Now()
			exec.FinishedAt = &now
		}
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if uc.runEventSink != nil {
			uc.runEventSink.OnRunFailed(exec.ctx, exec.ID, exec.GraphID, e.Error)
		}
		if err := uc.runRepo.UpdateRun(exec.ctx, persistSnap); err != nil {
			uc.lg.Warn("updateExecutionFromRuntimeEvent: UpdateRun failed for execution_error", loggateway.StepID("graph.record_fail"), loggateway.Str("execution_id", exec.ID), loggateway.Err(err))
		}
	case DomainEventGraphInterrupt:
		var persistSnap *GraphExecution
		exec.execMu.Lock()
		if exec.streamGen != gen {
			// Y2: stale stream's late interrupt must not pause the newer stream.
			exec.execMu.Unlock()
			return
		}
		exec.interruptMu.Lock()
		exec.interrupted = true
		exec.InterruptNode = e.NodeID
		exec.interruptMu.Unlock()
		uc.applyExecTransition(exec, GraphExecEventInterrupt)
		persistSnap = exec.SnapshotForPersist()
		exec.execMu.Unlock()
		if uc.runEventSink != nil {
			uc.runEventSink.OnRunWaitingApproval(exec.ctx, exec.ID, exec.GraphID, e.NodeID)
		}
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
// ISSUE-G2: execution exists but has no checkpoint lineage → (nil, nil)
// (200 空集) instead of ErrNotFound — "执行不存在"与"无检查点"两种状态可分。
func (uc *GraphExecutionUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (*GraphCheckpointState, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if exec.GraphID == "" || exec.LineageID == "" {
		return nil, nil
	}
	if err := uc.ensureCheckpointRuntime(ctx, exec); err != nil {
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
// ISSUE-G2: execution exists but has no checkpoint lineage → 空集 (200
// items: []) instead of ErrNotFound — "执行不存在"与"无检查点"两种状态可分。
func (uc *GraphExecutionUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) (GraphCheckpointList, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if exec.GraphID == "" || exec.LineageID == "" {
		return GraphCheckpointList{}, nil
	}
	if err := uc.ensureCheckpointRuntime(ctx, exec); err != nil {
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
	ticker := time.NewTicker(uc.gcConfig.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-uc.gcCancel:
			return
		case <-ticker.C:
			uc.gc()
		}
	}
}

// Close stops the GC goroutine and releases in-memory execution state.
func (uc *GraphExecutionUsecase) Close() {
	if uc == nil {
		return
	}
	select {
	case <-uc.gcCancel:
		// Already closed
	default:
		close(uc.gcCancel)
	}
}

func (uc *GraphExecutionUsecase) gc() {
	uc.mu.Lock()
	var expired []*GraphExecution
	now := time.Now()
	for id, exec := range uc.executions {
		exec.execMu.RLock()
		status := ParseGraphExecutionState(exec.Status)
		startedAt := exec.StartedAt
		var finishedAt *time.Time
		if exec.FinishedAt != nil {
			ft := *exec.FinishedAt
			finishedAt = &ft
		}
		rt := exec.runtime
		exec.execMu.RUnlock()

		if status.IsActive() {
			continue
		}
		if finishedAt != nil && now.Sub(*finishedAt) > uc.gcConfig.ExecutionMaxAge {
			exec.SetEvicted()
			delete(uc.executions, id)
			uc.cacheMgr.RemoveBuildConfig(id)
		} else if finishedAt == nil && now.Sub(startedAt) > uc.gcConfig.ExecutionMaxAge {
			if rt != nil {
				if err := rt.Cancel(); err != nil {
					uc.lg.Warn("cancel graph runtime on gc eviction", loggateway.Err(err))
				}
			}
			exec.execMu.Lock()
			// Try to transition to failed. If the execution is already in a terminal
			// state (completed/failed/cancelled), the transition is rejected and the
			// status is preserved — we still mark it evicted and clean up below.
			if err := uc.applyExecTransition(exec, GraphExecEventFail); err != nil {
				uc.lg.Warn("gc: execution already terminal, skipping fail transition",
					loggateway.StepID("graph.gc_already_terminal"),
					loggateway.Str("execution_id", exec.ID),
					loggateway.Str("status", exec.Status))
			}
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
