package agentbridge

import (
	"errors"
	"testing"
)

func TestTaskTransitionLegal(t *testing.T) {
	legal := []struct{ from, to TaskStatus }{
		{StatusDispatched, StatusRunning},
		{StatusDispatched, StatusFailed}, // start_failed
		{StatusRunning, StatusAwaitingApproval},
		{StatusAwaitingApproval, StatusRunning}, // permission_resolved
		{StatusAwaitingApproval, StatusCancelled}, // approval timeout
		{StatusRunning, StatusDone},
		{StatusRunning, StatusFailed},  // crash
		{StatusRunning, StatusCancelling},
		{StatusCancelling, StatusCancelled},
		{StatusCancelling, StatusFailed}, // cancel 过程中崩溃
		// 服务重启恢复：任何活跃态可转 failed
		{StatusDispatched, StatusFailed},
		{StatusAwaitingApproval, StatusFailed},
		{StatusCancelling, StatusFailed},
	}
	for _, tt := range legal {
		if err := Transition(tt.from, tt.to); err != nil {
			t.Errorf("Transition(%s→%s) should be legal, got %v", tt.from, tt.to, err)
		}
	}
}

func TestTaskTransitionIllegal(t *testing.T) {
	illegal := []struct{ from, to TaskStatus }{
		{StatusDispatched, StatusDone},   // 未完成不能完成
		{StatusDispatched, StatusCancelled}, // 未启动不能取消（只能 failed）
		{StatusDone, StatusRunning},      // 终态不可复活
		{StatusFailed, StatusRunning},
		{StatusCancelled, StatusRunning},
		{StatusRunning, StatusDispatched}, // 不可回退
		{StatusAwaitingApproval, StatusDone}, // 审批中不能直接完成
	}
	for _, tt := range illegal {
		if err := Transition(tt.from, tt.to); err == nil {
			t.Errorf("Transition(%s→%s) should be illegal", tt.from, tt.to)
		} else if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Transition(%s→%s) error should wrap ErrInvalidTransition, got %v", tt.from, tt.to, err)
		}
	}
}

func TestTaskTransitionSameStateNoop(t *testing.T) {
	// 同状态转换幂等允许（限流计数更新等场景复用 CAS）
	for _, s := range []TaskStatus{StatusDispatched, StatusRunning, StatusAwaitingApproval} {
		if err := Transition(s, s); err != nil {
			t.Errorf("same-state %s should be noop-legal, got %v", s, err)
		}
	}
}
