package agentbridge

import (
	"errors"
	"fmt"
)

// 保留理由（2026-08-20 P2-2 盘点）：不迁移到 biz/shared.GenericStateMachine——
// 本状态机为无事件 from→to 模型，且「同状态转换为幂等 noop（CAS 补丁复用）」
// 是 shared (from,event)→to 框架不支持的语义。AS-FSM-01 合规：显式转换表。

// ErrInvalidTransition 表示非法状态转换。
var ErrInvalidTransition = errors.New("agentbridge: invalid task state transition")

// transitions 合法转换表（AS-FSM-01）。键为 from，值为可达 to 集合。
// 同状态转换为幂等 noop（CAS 补丁复用场景）。
var transitions = map[TaskStatus]map[TaskStatus]bool{
	StatusDispatched: {
		StatusRunning: true,
		StatusFailed:  true, // start_failed / 服务重启恢复
	},
	StatusRunning: {
		StatusAwaitingApproval: true,
		StatusDone:             true,
		StatusFailed:           true,
		StatusCancelling:       true,
	},
	StatusAwaitingApproval: {
		StatusRunning:   true, // permission_resolved
		StatusCancelled: true, // 审批超时
		StatusFailed:    true, // 服务重启恢复
	},
	StatusCancelling: {
		StatusCancelled: true,
		StatusFailed:    true, // 取消过程中崩溃 / 服务重启恢复
	},
	// 终态无出边
	StatusDone:      {},
	StatusFailed:    {},
	StatusCancelled: {},
}

// Transition 校验 from→to 是否合法；合法返回 nil，非法返回包装 ErrInvalidTransition 的错误。
func Transition(from, to TaskStatus) error {
	if from == to && !from.IsTerminal() {
		return nil // 同状态幂等 noop
	}
	if transitions[from][to] {
		return nil
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
}
