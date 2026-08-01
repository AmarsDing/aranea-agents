package biz

import (
	"context"
	"strings"
	"time"
)

// Team 路径 graph 执行扩展（F-B）：team graph 路径以 GraphAgent 为 trpc
// runner 根，绕过 trpcGraphRuntime.Run，consumeRuntimeEvents 观察不到这些
// 执行。steps_json 增量落库与终态收敛由 team coordinator/runner 经下列方法
// 显式驱动（对齐 standalone 路径 node_end / 终态行为）。

// RecordTeamGraphNodeEnd upserts a per-node step snapshot for a team-linked
// graph execution and persists it immediately.
//
// Background (F-B): the team graph path runs the GraphAgent as the trpc runner
// root, bypassing trpcGraphRuntime.Run — so consumeRuntimeEvents never observes
// this execution and steps_json would stay NULL. The team coordinator's graph
// watch receives node lifecycle via system notices and mirrors the standalone
// path's node_end handling here.
func (uc *GraphExecutionUsecase) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	if uc == nil {
		return nil
	}
	exec, err := uc.loadExecution(ctx, strings.TrimSpace(execID))
	if err != nil {
		return err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	exec.execMu.Lock()
	exec.Steps = upsertGraphStep(exec.Steps, GraphStepSnapshot{
		NodeID:    strings.TrimSpace(nodeID),
		StepIndex: stepIndex,
		Status:    status,
		Error:     strings.TrimSpace(errMsg),
		Timestamp: time.Now(),
	})
	persistSnap := exec.SnapshotForPersist()
	persistCtx := exec.ctx
	exec.execMu.Unlock()
	if uc.runRepo == nil {
		return nil
	}
	if persistCtx == nil {
		persistCtx = context.Background()
	}
	return uc.runRepo.UpdateRun(persistCtx, persistSnap)
}

// FinalizeTeamGraphExecution converges a team-linked graph execution to a
// terminal state when the owning team run finishes. The team path has no
// runtime event consumer (F-B), so the runner/coordinator must close the
// execution explicitly. Idempotent: executions already in a terminal state
// are left untouched (a late duplicate finalize cannot flip the outcome).
func (uc *GraphExecutionUsecase) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	if uc == nil {
		return nil
	}
	exec, err := uc.loadExecution(ctx, strings.TrimSpace(execID))
	if err != nil {
		return err
	}
	exec.execMu.Lock()
	if IsGraphExecutionTerminal(ParseGraphExecutionState(exec.Status)) {
		exec.execMu.Unlock()
		return nil
	}
	event := GraphExecEventComplete
	if failed {
		event = GraphExecEventFail
	}
	if terr := uc.applyExecTransition(exec, event); terr != nil {
		exec.execMu.Unlock()
		return terr
	}
	now := time.Now()
	exec.FinishedAt = &now
	if failed {
		exec.ErrorMessage = strings.TrimSpace(errMsg)
	}
	persistSnap := exec.SnapshotForPersist()
	persistCtx := exec.ctx
	exec.execMu.Unlock()
	if uc.runRepo == nil {
		return nil
	}
	if persistCtx == nil {
		persistCtx = context.Background()
	}
	return uc.runRepo.UpdateRun(persistCtx, persistSnap)
}

// --- GraphUsecase 委托（team graph 路径） ---

func (uc *GraphUsecase) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return uc.execUC.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}

func (uc *GraphUsecase) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return uc.execUC.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
