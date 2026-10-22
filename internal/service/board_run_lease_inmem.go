package service

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
)

// InMemBoardRunLease 是 biz.BoardRunLeasePort 的进程内默认实现
// （P0-6 阶段 0，doNow ⑤）：语义包现有 PlanExecutor.running
// （boardRunRegistry，plan_executor.go:46-58）行为——LoadOrStore 先存者赢、
// Delete 幂等释放、Range 按 session 匹配活跃 dagRun。
//
// 边界声明：仅为阶段 1 PG 行租约迁移预留接缝；本阶段**未接线**（PlanExecutor
// 仍直接用 boardRunRegistry，零行为变更）。多副本部署下本实现不提供任何
// 跨进程去重——部署边界见 docs/development/replica-safety.md。
type InMemBoardRunLease struct {
	m sync.Map // boardID → *inMemLeaseEntry（对齐 boardRunRegistry：AS-COG-01 named wrapper）
}

type inMemLeaseEntry struct {
	lease     biz.BoardRunLease
	lastRenew time.Time // 进程内无语义；为阶段 1 心跳观测预留的接缝位
}

// NewInMemBoardRunLease 构造进程内默认实现。
func NewInMemBoardRunLease() *InMemBoardRunLease {
	return &InMemBoardRunLease{}
}

var _ biz.BoardRunLeasePort = (*InMemBoardRunLease)(nil)

// TryClaim 等价 boardRunRegistry.LoadOrStore：先存者赢；borderline
// （空 BoardID）必拒，claimed=false。
func (l *InMemBoardRunLease) TryClaim(_ context.Context, lease biz.BoardRunLease) (claimed bool) {
	if l == nil || lease.BoardID == "" {
		return false
	}
	_, loaded := l.m.LoadOrStore(lease.BoardID, &inMemLeaseEntry{
		lease:     lease,
		lastRenew: time.Now(),
	})
	return !loaded
}

// Renew 进程内实现仅刷新观测位；无活跃租约返回 false。
func (l *InMemBoardRunLease) Renew(_ context.Context, boardID string) bool {
	if l == nil || boardID == "" {
		return false
	}
	v, ok := l.m.Load(boardID)
	if !ok {
		return false
	}
	if e, ok := v.(*inMemLeaseEntry); ok {
		e.lastRenew = time.Now()
		return true
	}
	return false
}

// Release 等价 running.Delete：幂等，不存在时静默返回。
func (l *InMemBoardRunLease) Release(boardID string) {
	if l == nil || boardID == "" {
		return
	}
	l.m.Delete(boardID)
}

// HasActiveForSession 等价 PlanExecutor.HasActiveRunForSession：
// 任一活跃租约的 SessionID 匹配即 true；空 sessionID 恒 false。
func (l *InMemBoardRunLease) HasActiveForSession(sessionID string) bool {
	if l == nil || sessionID == "" {
		return false
	}
	active := false
	l.m.Range(func(_, v any) bool {
		if e, ok := v.(*inMemLeaseEntry); ok && e.lease.SessionID == sessionID {
			active = true
			return false
		}
		return true
	})
	return active
}

// ActiveLeaseCount 返回活跃租约数（测试/阶段 0 自检观测位，非端口成员）。
func (l *InMemBoardRunLease) ActiveLeaseCount() int {
	if l == nil {
		return 0
	}
	n := 0
	l.m.Range(func(_, _ any) bool { n++; return true })
	return n
}
