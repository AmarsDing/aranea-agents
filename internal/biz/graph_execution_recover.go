package biz

import (
	"context"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ---------------------------------------------------------------------------
// Crash recovery (83-长时运行韧性)
// ---------------------------------------------------------------------------

// RecoverOrphanedExecution 崩溃续跑入口（83-长时运行韧性，仅启动对账调用）。
func (uc *GraphUsecase) RecoverOrphanedExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	return uc.execUC.RecoverOrphanedExecution(ctx, executionID)
}

// RecoverOrphanedExecution 接管重启残留的 running 执行（仅启动对账路径调用）：
// 加载 DB 行 → HasCheckpoint 预检 → FSM recover 自环 → DefinitionHash 校验 →
// BuildAndResume(lineageID, nil) 重建 runtime → 恢复事件消费。
// 与 ResumeExecution 的区别：不要求 waiting_human、无 resumeValue、失败时不改写
// 执行状态（保持 running 原状），由调用方（team coordinator）决定回退判死。
func (uc *GraphExecutionUsecase) RecoverOrphanedExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	exec, err := uc.loadExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	exec.execMu.Lock()
	if _, err := uc.sm.Transition(ParseGraphExecutionState(exec.Status), GraphExecEventRecover); err != nil {
		exec.execMu.Unlock()
		return nil, ErrGraphInvalidStatus
	}
	lineageID := exec.LineageID
	exec.execMu.Unlock()

	// 安全闸：无 checkpoint 的恢复 = 从头重跑 = 副作用重复，拒绝。
	if lineageID == "" {
		return nil, ErrGraphCheckpointMissing
	}
	has, err := uc.factory.HasCheckpoint(ctx, lineageID)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, "graph checkpoint probe failed").WithCause(err)
	}
	if !has {
		return nil, ErrGraphCheckpointMissing
	}

	ct, err := uc.cacheMgr.BuildConfigForExecution(ctx, exec)
	if err != nil {
		return nil, err
	}

	// Y4 同款定义一致性校验：重启期间图定义被修改则拒绝续跑，调用方回退判死。
	exec.execMu.RLock()
	storedHash := exec.DefinitionHash
	exec.execMu.RUnlock()
	if storedHash != "" {
		if currentHash := ComputeGraphBuildConfigHash(ct.GraphBuildConfig); currentHash != storedHash {
			uc.lg.Warn("graph: crash recover rejected, definition changed since checkpoint",
				loggateway.StepID("graph.recover_hash_mismatch"),
				loggateway.Str("execution_id", executionID),
				loggateway.Str("graph_id", exec.GraphID))
			return nil, apierror.Conflict(apierror.DomainGraph, "graph definition changed since checkpoint; cannot crash-resume")
		}
	}

	// 同 ResumeExecution（L470 注释教训）：事件流脱离请求生命周期。
	runCtx := context.WithoutCancel(ctx)
	runtime, eventCh, err := uc.factory.BuildAndResume(runCtx, ct.GraphBuildConfig, exec.SessionID, exec.SpiritSessionID, exec.GraphID, executionID, lineageID, nil)
	if err != nil {
		e := apierror.Internal(apierror.DomainGraph, "graph crash recover failed")
		e.Cause = err
		return nil, e
	}

	exec.execMu.Lock()
	exec.runtime = runtime
	exec.ctx = runCtx
	exec.execMu.Unlock()

	// Y2 同款：先 bump stream generation 再起新消费，防旧 consumer 残留写状态。
	streamGen := exec.NextStreamGen()
	safego.GoBackground("graph.consumeEvents(recover)", func() {
		uc.consumeRuntimeEvents(eventCh, exec, streamGen, executionID, exec.GraphID, exec.SessionID, func() { uc.notifyExecComplete(exec) })
	})

	uc.lg.Info("graph: orphaned execution recovered from checkpoint",
		loggateway.StepID("graph.recover_ok"),
		loggateway.Str("execution_id", executionID),
		loggateway.Str("lineage_id", lineageID))
	return exec, nil
}
