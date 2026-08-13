package computeruse

import "testing"

func TestTransition_LegalPaths(t *testing.T) {
	cases := []struct {
		from SessionStatus
		ev   SessionEvent
		to   SessionStatus
	}{
		{SessionIdle, EvObserve, SessionObserving},
		{SessionIdle, EvGround, SessionGrounding},
		{SessionGrounding, EvAct, SessionActing},
		{SessionGrounding, EvAwaitConfirm, SessionAwaitingConfirm},
		{SessionAwaitingConfirm, EvConfirmed, SessionGrounding},
		{SessionActing, EvStepDone, SessionIdle},
		{SessionGrounding, EvStepDone, SessionIdle}, // 干跑
		{SessionIdle, EvFinish, SessionDone},
		{SessionActing, EvFail, SessionFailed},
		{SessionActing, EvCancel, SessionCancelled},
		{SessionAwaitingConfirm, EvCancel, SessionCancelled},
		// 75 review B3：以下转换原本被直赋绕过，现纳入状态机表。
		{SessionIdle, EvFail, SessionFailed},          // 预算耗尽（beginStep 于 idle 判失败）
		{SessionObserving, EvFinish, SessionDone},     // 用户中途结束
		{SessionGrounding, EvFinish, SessionDone},     // 用户中途结束
		{SessionActing, EvFinish, SessionDone},        // 用户中途结束
		{SessionAwaitingConfirm, EvFinish, SessionDone}, // 用户中途结束
	}
	for _, c := range cases {
		got, err := Transition(c.from, c.ev)
		if err != nil {
			t.Errorf("Transition(%s,%s) unexpected err: %v", c.from, c.ev, err)
			continue
		}
		if got != c.to {
			t.Errorf("Transition(%s,%s) = %s, want %s", c.from, c.ev, got, c.to)
		}
	}
}

func TestTransition_IllegalRejected(t *testing.T) {
	cases := []struct {
		from SessionStatus
		ev   SessionEvent
	}{
		{SessionDone, EvGround},         // 终态不可再转换
		{SessionCancelled, EvAct},       // 急停后不可动作
		{SessionFailed, EvStepDone},     // 失败终态
		{SessionIdle, EvStepDone},       // idle 无单步完成
		{SessionObserving, EvAct},       // 感知中不能直接动作
		{SessionAwaitingConfirm, EvAct}, // 确认等待中不能动作
	}
	for _, c := range cases {
		if _, err := Transition(c.from, c.ev); err == nil {
			t.Errorf("Transition(%s,%s) should be rejected", c.from, c.ev)
		}
		if CanTransition(c.from, c.ev) {
			t.Errorf("CanTransition(%s,%s) should be false", c.from, c.ev)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	for _, s := range []SessionStatus{SessionDone, SessionFailed, SessionCancelled} {
		if !IsTerminal(s) {
			t.Errorf("IsTerminal(%s) should be true", s)
		}
	}
	for _, s := range []SessionStatus{SessionIdle, SessionObserving, SessionGrounding, SessionActing, SessionAwaitingConfirm} {
		if IsTerminal(s) {
			t.Errorf("IsTerminal(%s) should be false", s)
		}
	}
}
