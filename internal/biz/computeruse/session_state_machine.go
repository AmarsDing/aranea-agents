package computeruse

import "fmt"

// SessionEvent 状态机事件。
type SessionEvent string

const (
	EvObserve      SessionEvent = "observe"       // 开始感知
	EvGround       SessionEvent = "ground"        // 开始 grounding
	EvAct          SessionEvent = "act"           // 开始动作注入
	EvAwaitConfirm SessionEvent = "await_confirm" // 进入确认等待
	EvConfirmed    SessionEvent = "confirmed"     // 确认通过，回到 grounding
	EvStepDone     SessionEvent = "step_done"     // 单步完成，回到 idle
	EvFinish       SessionEvent = "finish"        // 正常结束
	EvFail         SessionEvent = "fail"          // 失败
	EvCancel       SessionEvent = "cancel"        // 急停/超预算取消
)

// transitions 合法转换表：from → event → to。
var transitions = map[SessionStatus]map[SessionEvent]SessionStatus{
	SessionIdle: {
		EvObserve: SessionObserving,
		EvGround:  SessionGrounding,
		EvCancel:  SessionCancelled,
		EvFinish:  SessionDone,
	},
	SessionObserving: {
		EvStepDone: SessionIdle,
		EvFail:     SessionFailed,
		EvCancel:   SessionCancelled,
	},
	SessionGrounding: {
		EvAct:          SessionActing,
		EvAwaitConfirm: SessionAwaitingConfirm,
		EvStepDone:     SessionIdle, // 干跑：grounding 完即回 idle
		EvFail:         SessionFailed,
		EvCancel:       SessionCancelled,
	},
	SessionAwaitingConfirm: {
		EvConfirmed: SessionGrounding,
		EvCancel:    SessionCancelled, // 确认拒绝/超时
	},
	SessionActing: {
		EvStepDone: SessionIdle,
		EvFail:     SessionFailed,
		EvCancel:   SessionCancelled,
	},
	// 终态：done/failed/cancelled 不允许再转换（会话复用由 Usecase 重建 idle 处理）
}

// Transition 校验并返回目标状态；非法转换返回错误。
func Transition(from SessionStatus, ev SessionEvent) (SessionStatus, error) {
	if evs, ok := transitions[from]; ok {
		if to, ok := evs[ev]; ok {
			return to, nil
		}
	}
	return "", fmt.Errorf("computeruse: 非法状态转换 %s --%s-->", from, ev)
}

// CanTransition 仅校验不转换。
func CanTransition(from SessionStatus, ev SessionEvent) bool {
	_, err := Transition(from, ev)
	return err == nil
}

// IsTerminal 判断是否终态。
func IsTerminal(s SessionStatus) bool {
	return s == SessionDone || s == SessionFailed || s == SessionCancelled
}
