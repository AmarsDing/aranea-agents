package biz

import (
	"context"
)

// ============================================================================
// BoardRunLeasePort —— PlanBoard DAG 执行租约窄接口（P0-6 阶段 0，doNow ⑤）
// ============================================================================
//
// 背景与边界（docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §7）：
// 当前 PlanExecutor 的 DAG 执行租约驻留进程内存（boardRunRegistry，
// internal/service/plan_executor.go:46-58），多副本下同一
// PlanBoardCreatedEvent 会被多个实例各自认领造成重复派发。本端口把租约
// 语义显式化为窄接口，阶段 0 仅提供进程内默认实现
// （internal/service/board_run_lease_inmem.go，零行为变更、未接线）；
// 阶段 1 迁移为 PG 行租约（owner + expires_at + 心跳，复用
// ChannelRuntimeLeaseRepo / ClaimPendingDeliveries SKIP LOCKED 先例），
// 全程藏 capability flag。部署边界见 docs/development/replica-safety.md。
//
// 单副本边界：进程内 lease 无心跳、无过期、无跨进程认领——语义等价于"本进程
// 唯一"，多副本部署必须经 replica-safety 启动自检的 Warn 降级承认。
//
// Stability: internal（阶段 1 迁移落地后转 evolving）
type BoardRunLeasePort interface {
	// TryClaim 原子认领 board 的执行租约（等价现有
	// boardRunRegistry.LoadOrStore：先存者赢，后到调用方负责取消自己多余的
	// lease.Cancel）。claimed=false 表示已有活跃持有方。
	TryClaim(ctx context.Context, lease BoardRunLease) (claimed bool)

	// Renew 续约租约。进程内实现无过期语义，仅刷新 lastRenew 观测位
	// （阶段 1 PG 实现将写 owner+expires_at 心跳）。未发现活跃租约返回 false。
	Renew(ctx context.Context, boardID string) bool

	// Release 释放租约（等价现有 running.Delete）。幂等：不存在时静默返回。
	Release(boardID string)

	// HasActiveForSession 报告任一活跃租约是否属于给定 spirit session
	// （等价现有 HasActiveRunForSession，synthesis 波次中点门控）。
	HasActiveForSession(sessionID string) bool
}

// BoardRunLease 是一次 DAG 租约的载体（对齐内部 boardRunLease 字段语义）：
// Cancel 由持有者用于 CancelPlanBoard（C-18）；SessionID 供
// HasActiveForSession 按 spirit session 匹配活跃 dagRun。
type BoardRunLease struct {
	BoardID   string
	SessionID string
	Cancel    context.CancelFunc
}
