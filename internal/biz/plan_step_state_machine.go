package biz

import "aranea-agents/pkg/apierror"

// 保留理由（2026-08-20 P2-2 盘点）：不迁移到 biz/shared.GenericStateMachine——
// 本状态机为无事件 to-list 模型且转换方法是 PlanStep 实体方法（直接赋值 s.Status），
// 迁入 (from,event)→to 框架需伪造事件并改实体 API，反而增加复杂度；非法转换须
// 返回 apierror.BadRequest 保证 400 映射。AS-FSM-01 合规：显式转换表 + 禁止直赋。

// planStepTransitions 定义 PlanStep 的合法状态转换表（spec §3.5.4）。
// key = 源状态，value = 可到达的目标状态列表。
var planStepTransitions = map[PlanStepStatus][]PlanStepStatus{
	PlanStepStatusPending: {
		PlanStepStatusRunning,
		PlanStepStatusSkipped, // 依赖失败时跳过
	},
	PlanStepStatusRunning: {
		PlanStepStatusCompleted,
		PlanStepStatusFailed,
		PlanStepStatusSkipped,
		PlanStepStatusPartialFailure,
	},
	PlanStepStatusCompleted:      {},                      // terminal
	PlanStepStatusFailed:         {PlanStepStatusRunning}, // 允许重试
	PlanStepStatusSkipped:        {},                      // terminal
	PlanStepStatusPartialFailure: {PlanStepStatusRunning}, // 允许重试
}

// Transition 校验并执行状态转换。禁止跳过状态机直接赋值（spec §3.5.4）。
func (s *PlanStep) Transition(to PlanStepStatus) error {
	allowed, ok := planStepTransitions[s.Status]
	if !ok {
		return apierror.BadRequest(apierror.DomainSession, "unknown source status: %s", s.Status)
	}
	for _, a := range allowed {
		if a == to {
			s.Status = to
			return nil
		}
	}
	return apierror.BadRequest(apierror.DomainSession, "invalid transition: %s → %s", s.Status, to)
}

// CanTransition 返回是否可以从当前状态转换到目标状态（不执行转换）。
func (s *PlanStep) CanTransition(to PlanStepStatus) bool {
	allowed, ok := planStepTransitions[s.Status]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// IsTerminal 返回当前状态是否为终态。
func (s PlanStepStatus) IsTerminal() bool {
	switch s {
	case PlanStepStatusCompleted, PlanStepStatusSkipped:
		return true
	default:
		return false
	}
}
