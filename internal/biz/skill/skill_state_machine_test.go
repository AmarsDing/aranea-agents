package skill

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

// TestSkillStateMachine_TransitionMatrix 钉死全状态×全事件矩阵（AS-FSM-01：
// 迁移表驱动，非法迁移返回 ErrInvalidTransition）。
func TestSkillStateMachine_TransitionMatrix(t *testing.T) {
	sm := NewSkillStateMachine()

	type expectation struct {
		to SkillState
		ok bool
	}
	states := []SkillState{
		SkillStateDraft, SkillStateValidated, SkillStateApproved,
		SkillStateApplied, SkillStateRolledBack,
	}
	events := []SkillEvent{
		SkillEventValidate, SkillEventApprove, SkillEventApply, SkillEventRollback,
	}
	want := map[SkillState]map[SkillEvent]expectation{
		SkillStateDraft: {
			SkillEventValidate: {SkillStateValidated, true},
			SkillEventApprove:  {"", false},
			SkillEventApply:    {"", false},
			SkillEventRollback: {"", false},
		},
		SkillStateValidated: {
			SkillEventValidate: {"", false},
			SkillEventApprove:  {SkillStateApproved, true},
			SkillEventApply:    {"", false},
			SkillEventRollback: {"", false},
		},
		SkillStateApproved: {
			SkillEventValidate: {"", false},
			SkillEventApprove:  {"", false},
			SkillEventApply:    {SkillStateApplied, true},
			SkillEventRollback: {"", false},
		},
		SkillStateApplied: {
			SkillEventValidate: {"", false},
			SkillEventApprove:  {"", false},
			SkillEventApply:    {"", false},
			SkillEventRollback: {SkillStateRolledBack, true},
		},
		SkillStateRolledBack: {
			SkillEventValidate: {"", false},
			SkillEventApprove:  {"", false},
			SkillEventApply:    {"", false},
			SkillEventRollback: {"", false},
		},
	}

	for _, from := range states {
		for _, ev := range events {
			exp := want[from][ev]
			to, err := sm.Transition(from, ev)
			if exp.ok {
				if err != nil {
					t.Fatalf("from=%s event=%s: expected success, got err=%v", from, ev, err)
				}
				if to != exp.to {
					t.Fatalf("from=%s event=%s: expected to=%s, got=%s", from, ev, exp.to, to)
				}
			} else {
				if !errors.Is(err, shared.ErrInvalidTransition) {
					t.Fatalf("from=%s event=%s: expected ErrInvalidTransition, got=%v", from, ev, err)
				}
			}
		}
	}
}

// TestSkillStateMachine_TransitionFromString 钉死字符串入口的 fail-closed：
// 未知源状态一律 ErrInvalidTransition，绝不放行。
func TestSkillStateMachine_TransitionFromString(t *testing.T) {
	sm := NewSkillStateMachine()

	if to, err := sm.TransitionFromString("draft", SkillEventValidate); err != nil || to != SkillStateValidated {
		t.Fatalf("draft+validate: to=%s err=%v", to, err)
	}
	for _, bogus := range []string{"", "published", "active", "archived", "bogus", "DRAFT"} {
		if _, err := sm.TransitionFromString(bogus, SkillEventValidate); !errors.Is(err, shared.ErrInvalidTransition) {
			t.Fatalf("from=%q: expected ErrInvalidTransition, got=%v", bogus, err)
		}
	}
	// 未知事件同样拒绝。
	if _, err := sm.TransitionFromString("draft", SkillEvent("publish")); !errors.Is(err, shared.ErrInvalidTransition) {
		t.Fatalf("unknown event: expected ErrInvalidTransition, got=%v", err)
	}
}

// TestSkillStateMachine_CanTransitionAndValidTargets 核查可达性查询面。
func TestSkillStateMachine_CanTransitionAndValidTargets(t *testing.T) {
	sm := NewSkillStateMachine()

	if !sm.CanTransition(SkillStateDraft, SkillStateValidated) {
		t.Fatal("expected draft→validated allowed")
	}
	if sm.CanTransition(SkillStateDraft, SkillStateApplied) {
		t.Fatal("expected draft→applied disallowed")
	}
	if targets := sm.ValidTargets(SkillStateApproved); len(targets) != 1 || targets[0] != SkillStateApplied {
		t.Fatalf("expected approved targets=[applied], got=%v", targets)
	}
	if got := sm.ValidTargets(SkillStateRolledBack); got != nil {
		t.Fatalf("expected rolled_back terminal targets=nil, got=%v", got)
	}
}

// TestSkillStateMachine_Terminal 钉死终态集合。
func TestSkillStateMachine_Terminal(t *testing.T) {
	if !IsSkillTerminalState(SkillStateRolledBack) {
		t.Fatal("expected rolled_back terminal")
	}
	for _, s := range []SkillState{SkillStateDraft, SkillStateValidated, SkillStateApproved, SkillStateApplied} {
		if IsSkillTerminalState(s) {
			t.Fatalf("expected %s non-terminal", s)
		}
	}
}

// TestSkillTransitionEventLevel 钉死 AS-EVT-01 事件分级（设计 §9.3）：
// applied/rolled_back=Important，validated/approved=Informational。
func TestSkillTransitionEventLevel(t *testing.T) {
	cases := []struct {
		event SkillEvent
		want  SkillEventLevel
	}{
		{SkillEventApply, SkillEventLevelImportant},
		{SkillEventRollback, SkillEventLevelImportant},
		{SkillEventValidate, SkillEventLevelInformational},
		{SkillEventApprove, SkillEventLevelInformational},
	}
	for _, c := range cases {
		if got := SkillTransitionEventLevel(c.event); got != c.want {
			t.Fatalf("event=%s: expected level=%s, got=%s", c.event, c.want, got)
		}
	}
}

// TestSkillStateMachine_TransitionAudit 钉死「非法迁移拒绝+事件」：
// 允许与拒绝的跃迁都必须产生审计记录，且分级正确。
func TestSkillStateMachine_TransitionAudit(t *testing.T) {
	sm := NewSkillStateMachine()
	var recs []SkillTransitionRecord
	sink := func(r SkillTransitionRecord) { recs = append(recs, r) }

	// 合法跃迁：audit 记录 Allowed=true + 事件分级。
	to, err := sm.TransitionAudit(SkillStateApproved, SkillEventApply, "skill_1", sink)
	if err != nil || to != SkillStateApplied {
		t.Fatalf("expected apply success, to=%s err=%v", to, err)
	}
	// 非法跃迁：audit 记录 Allowed=false + Informational。
	if _, err := sm.TransitionAudit(SkillStateDraft, SkillEventApply, "skill_1", sink); !errors.Is(err, shared.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got=%v", err)
	}
	// nil sink 安全。
	if _, err := sm.TransitionAudit(SkillStateDraft, SkillEventValidate, "skill_1", nil); err != nil {
		t.Fatalf("nil sink: unexpected err=%v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 audit records, got=%d", len(recs))
	}
	if !recs[0].Allowed || recs[0].Level != SkillEventLevelImportant || recs[0].To != SkillStateApplied {
		t.Fatalf("bad allowed record: %+v", recs[0])
	}
	if recs[1].Allowed || recs[1].Level != SkillEventLevelInformational {
		t.Fatalf("bad denied record: %+v", recs[1])
	}
}

// TestMapSkillStateToLegacy 钉死 status/lifecycle_status 双字段映射表
// （评审注 ①：灭绝双字段各自为政的根源歧义；未知状态 fail-closed）。
func TestMapSkillStateToLegacy(t *testing.T) {
	cases := []struct {
		state     SkillState
		status    string
		lifecycle string
		ok        bool
	}{
		{SkillStateDraft, "draft", "draft", true},
		{SkillStateValidated, "draft", "validated", true},
		{SkillStateApproved, "draft", "approved", true},
		{SkillStateApplied, "published", "applied", true},
		{SkillStateRolledBack, "archived", "rolled_back", true},
		{SkillState("bogus"), "", "", false},
		{SkillState(""), "", "", false},
	}
	for _, c := range cases {
		status, lifecycle, ok := MapSkillStateToLegacy(c.state)
		if status != c.status || lifecycle != c.lifecycle || ok != c.ok {
			t.Fatalf("state=%q: expected (%q,%q,%v), got (%q,%q,%v)",
				c.state, c.status, c.lifecycle, c.ok, status, lifecycle, ok)
		}
	}
}

// TestParseSkillState 核查解析面。
func TestParseSkillState(t *testing.T) {
	if s, ok := ParseSkillState("validated"); !ok || s != SkillStateValidated {
		t.Fatalf("expected validated parsed, got=%q ok=%v", s, ok)
	}
	if _, ok := ParseSkillState("published"); ok {
		t.Fatal("expected published rejected as canonical state")
	}
}
