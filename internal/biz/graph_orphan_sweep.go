package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// D6-F4（87-orchestration-chain-hardening）：standalone 孤儿执行启动终态化扫描
// ---------------------------------------------------------------------------

// orphanStandaloneReason 是启动扫描终态化 standalone 孤儿执行的失败原因
// （对齐 RestartFailureReason 系常量语义：进程重启遗留）。
const orphanStandaloneReason = "server restart: orphaned graph execution finalized"

// orphanSweepScanLimit 是单次启动扫描的行数上限；超出部分留待进程内
// GC/对账兜底（终态守卫幂等，不构成泄漏）。
const orphanSweepScanLimit = 500

// FinalizeOrphanedStandaloneExecutions 启动对账（D6-F4）：把没有
// team_graph_sessions 行的 running standalone graph_executions 收敛为
// failed。team 行由 RecoverSessions/team_orphan_recovery 终态化
// （ADR-86 F-B 边界铁律），本扫描只做终态化、不续跑（standalone 无 resume
// 入口责任人）。
//
// 幂等：FinalizeTeamGraphExecution 先 FSM 终态守卫，二次启动/多副本交叠
// 安全（多副本同时 finalize 同一行结果同值覆盖）。扫描器缺失（runRepo nil
// 或未实现 GraphOrphanRunScanner 窄接口）时幂等空转返回 (0, nil)。逐条失败
// 仅 Warn 并累计，不中断；整体扫描失败返回错误由调用方决定告警（启动路径
// 非致命）。
func (uc *GraphExecutionUsecase) FinalizeOrphanedStandaloneExecutions(ctx context.Context) (int, error) {
	if uc == nil || uc.runRepo == nil {
		return 0, nil
	}
	scanner, ok := uc.runRepo.(GraphOrphanRunScanner)
	if !ok {
		return 0, nil
	}
	orphans, err := scanner.ListOrphanedStandaloneRuns(ctx, string(GraphExecRunning), orphanSweepScanLimit)
	if err != nil {
		return 0, err
	}
	finalized := 0
	failures := 0
	for _, orphan := range orphans {
		if orphan == nil {
			continue
		}
		// 防御：scanner 契约已按 status=running 过滤；waiting_human /
		// completed / failed / cancelled 一律不动（waiting_human 归 HITL
		// 恢复通道，见 FinalizeTeamGraphExecution 的 F7 语义）。
		if ParseGraphExecutionState(strings.TrimSpace(orphan.Status)) != GraphExecRunning {
			continue
		}
		if err := uc.FinalizeTeamGraphExecution(ctx, orphan.ID, true, orphanStandaloneReason); err != nil {
			failures++
			uc.lg.Warn("graph: orphaned standalone execution finalize failed",
				loggateway.StepID("graph.orphan_sweep_fail"),
				loggateway.Str("exec_id", orphan.ID),
				loggateway.Str("graph_id", orphan.GraphID),
				loggateway.Err(err))
			continue
		}
		finalized++
	}
	if finalized > 0 || failures > 0 {
		uc.lg.Info("graph: orphaned standalone executions finalized on startup",
			loggateway.StepID("graph.orphan_sweep"),
			loggateway.Int("finalized", finalized),
			loggateway.Int("failed", failures),
			loggateway.Int("scanned", len(orphans)))
	}
	return finalized, nil
}

// FinalizeOrphanedStandaloneExecutions 委托到 GraphExecutionUsecase（D6-F4）。
func (uc *GraphUsecase) FinalizeOrphanedStandaloneExecutions(ctx context.Context) (int, error) {
	return uc.execUC.FinalizeOrphanedStandaloneExecutions(ctx)
}
