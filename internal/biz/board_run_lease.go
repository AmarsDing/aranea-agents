package biz

import (
	"context"
	"time"
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

// ============================================================================
// BoardRunLeaseRepo —— PG 行租约持久化窄口（P0-6 阶段 1a，
// 90-multi-replica-board-lease）
// ============================================================================
//
// 由 data 侧（internal/data/board_run_lease_repo.go，迁移 20261295
// board_run_leases 表）实现、service 侧 PGBoardRunLease 消费——repo 保持
// 纯 SQL 无心跳驱动（心跳属 service 实例级职责）。
//
// 与 ChannelRuntimeLeaseRepo 收养先例的唯一差异：TryClaim 只允许**过期行
// 收养**，不允许同 owner 收养——同进程二次 startBoardDAG(board) 必须
// claimed=false（否则单进程内重复建团，丢失 C-20 幂等）。
//
// 时间列为 TEXT + 定宽布局（"2006-01-02T15:04:05.000000000Z07:00"，纳秒
// 恒 9 位）——对抗评审 #0/#11：RFC3339Nano 变宽小数在整秒瞬时字典序倒置
// （"…:05Z" > "…:05.9Z" 但时序更早），会误收养活跃租约/漏判 HasActive。
// Stability: internal
type BoardRunLeaseRepo interface {
	// TryClaimBoardRunLease 原子认领：INSERT ... ON CONFLICT DO UPDATE WHERE
	// expires_at<=now（仅过期收养）。claimed=false（nil error）=被活跃持有。
	// ttl>0 必填；lease.Cancel 不持久化（进程内载体，仅 BoardID/SessionID 落行）。
	TryClaimBoardRunLease(ctx context.Context, lease BoardRunLease, ownerID string, ttl time.Duration) (claimed bool, err error)

	// RenewBoardRunLease owner+未过期双守卫续约；renewed=false（nil error）=
	// 租约丢失/过期/他人持有。
	RenewBoardRunLease(ctx context.Context, boardID, ownerID string, ttl time.Duration) (renewed bool, err error)

	// ReleaseBoardRunLease owner-scoped 释放（永不删他人行）；幂等。
	ReleaseBoardRunLease(ctx context.Context, boardID, ownerID string) error

	// HasActiveBoardRunForSession 报告 session 是否存在未过期租约
	// （跨副本波次中点门控数据源）。
	HasActiveBoardRunForSession(ctx context.Context, sessionID string, now time.Time) (active bool, err error)
}
