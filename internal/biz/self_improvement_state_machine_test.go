package biz

import "testing"

// TestSelfImprovementRunSM_Transitions covers the full transition table of the
// platform self-improvement run state machine (design D3).
func TestSelfImprovementRunSM_Transitions(t *testing.T) {
	sm := NewSelfImprovementRunStateMachine()

	cases := []struct {
		name    string
		from    SelfImprovementRunStatus
		event   SelfImprovementRunEvent
		want    SelfImprovementRunStatus
		wantErr bool
	}{
		// 正常路径
		{"detect→diagnose", RunStatusDetected, RunEventDiagnose, RunStatusDiagnosing, false},
		{"diagnose→patch", RunStatusDiagnosing, RunEventPatch, RunStatusPatching, false},
		{"diagnose→record_only", RunStatusDiagnosing, RunEventRecordOnly, RunStatusClosed, false},
		{"patch→verify", RunStatusPatching, RunEventVerify, RunStatusVerifying, false},
		{"verify→pass", RunStatusVerifying, RunEventVerifyPass, RunStatusAwaitingGovernance, false},
		{"govern→apply", RunStatusAwaitingGovernance, RunEventApply, RunStatusApplying, false},
		{"apply→done", RunStatusApplying, RunEventApplyDone, RunStatusApplied, false},
		{"applied→observe", RunStatusApplied, RunEventObserve, RunStatusObserving, false},
		{"observe→close", RunStatusObserving, RunEventClose, RunStatusClosed, false},
		// 合并冲突转人工（design D7：冲突则转人工）
		{"apply→escalate→govern", RunStatusApplying, RunEventApplyEscalate, RunStatusAwaitingGovernance, false},
		// 验证失败重试回路
		{"verify→retry→patch", RunStatusVerifying, RunEventVerifyFail, RunStatusPatching, false},
		{"verify→fail_final", RunStatusVerifying, RunEventVerifyFailFinal, RunStatusVerifyFailed, false},
		// 审批拒绝
		{"govern→reject", RunStatusAwaitingGovernance, RunEventReject, RunStatusRejected, false},
		// 策略拒绝（保护文件/敏感内容/超规模 diff 的 fail-fast，design D9/D10）
		{"patching→reject", RunStatusPatching, RunEventReject, RunStatusRejected, false},
		{"verifying→reject", RunStatusVerifying, RunEventReject, RunStatusRejected, false},
		// 回滚
		{"observing→rollback", RunStatusObserving, RunEventRollback, RunStatusRolledBack, false},
		{"applied→rollback", RunStatusApplied, RunEventRollback, RunStatusRolledBack, false},
		// 异常
		{"applying→error", RunStatusApplying, RunEventError, RunStatusFailed, false},
		{"patching→error", RunStatusPatching, RunEventError, RunStatusFailed, false},
		// 非法迁移
		{"detected不能跳verify", RunStatusDetected, RunEventVerify, "", true},
		{"closed终态不可迁移", RunStatusClosed, RunEventRollback, "", true},
		{"rolled_back终态不可迁移", RunStatusRolledBack, RunEventApply, "", true},
		{"rejected终态不可迁移", RunStatusRejected, RunEventApply, "", true},
		{"verify_failed终态不可迁移", RunStatusVerifyFailed, RunEventPatch, "", true},
		{"failed终态不可迁移", RunStatusFailed, RunEventDiagnose, "", true},
		{"observing不能apply", RunStatusObserving, RunEventApply, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sm.Transition(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Transition(%s,%s) 期望错误，实际成功到 %s", tc.from, tc.event, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Transition(%s,%s) 意外错误: %v", tc.from, tc.event, err)
			}
			if got != tc.want {
				t.Fatalf("Transition(%s,%s) = %s，期望 %s", tc.from, tc.event, got, tc.want)
			}
		})
	}
}

// TestSelfImprovementRunSM_TerminalStates asserts terminal states have no
// outgoing transitions.
func TestSelfImprovementRunSM_TerminalStates(t *testing.T) {
	sm := NewSelfImprovementRunStateMachine()
	terminals := []SelfImprovementRunStatus{
		RunStatusClosed, RunStatusVerifyFailed, RunStatusRolledBack, RunStatusRejected, RunStatusFailed,
	}
	for _, s := range terminals {
		if targets := sm.ValidTargets(s); len(targets) != 0 {
			t.Errorf("终态 %s 不应有可达状态，实际: %v", s, targets)
		}
		if !IsSelfImprovementRunTerminal(s) {
			t.Errorf("IsSelfImprovementRunTerminal(%s) 应为 true", s)
		}
	}
	nonTerminals := []SelfImprovementRunStatus{
		RunStatusDetected, RunStatusDiagnosing, RunStatusPatching, RunStatusVerifying,
		RunStatusAwaitingGovernance, RunStatusApplying, RunStatusApplied, RunStatusObserving,
	}
	for _, s := range nonTerminals {
		if IsSelfImprovementRunTerminal(s) {
			t.Errorf("IsSelfImprovementRunTerminal(%s) 应为 false", s)
		}
	}
}
