// Package skill — Skill governance State Machine (AS-FSM-01, P0-7 A14)
//
// This file defines the explicit state machine for the skill governance
// lifecycle (draft → validated → approved → applied → rolled_back), per the
// reviewed design in
// docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §8.4①.
//
//	/*
//	```mermaid
//	stateDiagram-v2
//	  [*] --> Draft
//	  Draft --> Validated : validate
//	  Validated --> Approved : approve
//	  Approved --> Applied : apply
//	  Applied --> RolledBack : rollback
//	  RolledBack --> [*]
//	```
//	*/
//
// 双字段映射（评审注 ①）：canonical 状态是治理生命周期的唯一真相源；
// platform_skill.status（粗粒度 draft/published/archived，"active" 为
// published 的历史别名）与 platform_skill.lifecycle_status 均由
// MapSkillStateToLegacy 推导，禁止两字段各自为政。
//
// 事件分级（AS-EVT-01，设计 §9.3）：
//   - skill.applied / skill.rolled_back → Important（BlockUpTo+异步持久化）
//   - skill.validated / skill.approved → Informational（丢失仅降可观测性；
//     approve 的人工决策本身已由 learning_loop decision records 审计）
//   - skill.trust_changed → Important；skill.trust_filtered → Informational
//     （信任维度事件，常量定义于 internal/biz/skill_trust.go）
//
// wiring 说明：本机件为纯状态机 + 审计记录面；usecase 公开入口
// （Publish/Approve/Apply）的 FSM 校验接线与事件出站（总线/WBPF）随
// ent 列与回填迁移一并在 Batch B 接入，本轮（去 ent 版）零行为变更。
package skill

import (
	"aranea-agents/internal/biz/shared"
)

// ── Skill governance State & Event types ─────────────────────────────────────

// SkillState enumerates all legal states of the skill governance lifecycle.
// String values are the canonical stored form for lifecycle_status.
//
// Stability:stable
type SkillState string

const (
	SkillStateDraft      SkillState = "draft"
	SkillStateValidated  SkillState = "validated"
	SkillStateApproved   SkillState = "approved"
	SkillStateApplied    SkillState = "applied"
	SkillStateRolledBack SkillState = "rolled_back"
)

// SkillEvent enumerates events that trigger a skill governance transition.
//
// Stability:stable
type SkillEvent string

const (
	SkillEventValidate SkillEvent = "validate"
	SkillEventApprove  SkillEvent = "approve"
	SkillEventApply    SkillEvent = "apply"
	SkillEventRollback SkillEvent = "rollback"
)

// SkillEventLevel is the AS-EVT-01 grading of a governance event.
type SkillEventLevel string

const (
	// SkillEventLevelImportant — BlockUpTo+异步持久化（Critical/Important 车道语义见 §9.3）。
	SkillEventLevelImportant SkillEventLevel = "important"
	// SkillEventLevelInformational — 尽力而为；丢失仅降可观测性。
	SkillEventLevelInformational SkillEventLevel = "informational"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// skillTransitionRules defines the complete legal transition table.
// RolledBack is terminal (no outgoing transitions). 未列出的 (from, event)
// 组合一律非法——Transition 返回 shared.ErrInvalidTransition 并产生审计记录。
var skillTransitionRules = []shared.TransitionRule[SkillState, SkillEvent]{
	{From: SkillStateDraft, Event: SkillEventValidate, To: SkillStateValidated},
	{From: SkillStateValidated, Event: SkillEventApprove, To: SkillStateApproved},
	{From: SkillStateApproved, Event: SkillEventApply, To: SkillStateApplied},
	{From: SkillStateApplied, Event: SkillEventRollback, To: SkillStateRolledBack},
}

// ── SkillStateMachine ────────────────────────────────────────────────────────

// SkillStateMachine wraps the generic state machine with skill governance
// types. Safe for concurrent use after construction (immutable rule table).
//
// Stability:stable
type SkillStateMachine struct {
	inner *shared.GenericStateMachine[SkillState, SkillEvent]
}

// NewSkillStateMachine creates a SkillStateMachine with the standard rules.
func NewSkillStateMachine() *SkillStateMachine {
	return &SkillStateMachine{
		inner: shared.NewGenericStateMachine[SkillState, SkillEvent](skillTransitionRules),
	}
}

// Transition validates and executes a state transition. 非法迁移返回
// shared.ErrInvalidTransition（unknown from 或 from×event 无规则）。
func (sm *SkillStateMachine) Transition(from SkillState, event SkillEvent) (SkillState, error) {
	return sm.inner.Transition(from, event)
}

// TransitionFromString is Transition for raw stored strings. Unknown source
// states fail closed with shared.ErrInvalidTransition — 绝不对未知状态放行。
func (sm *SkillStateMachine) TransitionFromString(from string, event SkillEvent) (SkillState, error) {
	return sm.inner.Transition(SkillState(from), event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *SkillStateMachine) CanTransition(from, to SkillState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state.
func (sm *SkillStateMachine) ValidTargets(from SkillState) []SkillState {
	return sm.inner.ValidTargets(from)
}

// SkillTransitionRecord is the audit record for one transition attempt
// (allowed OR denied). Denied records carry the zero To and Allowed=false.
type SkillTransitionRecord struct {
	SkillID string
	From    SkillState
	Event   SkillEvent
	To      SkillState
	Allowed bool
	// Level 按 AS-EVT-01 给出该记录的最小定级；denied 记录恒 Informational
	// （非法迁移由错误返回表达拒绝，事件仅作观测，不上 BlockUpTo 车道放大）。
	Level SkillEventLevel
}

// TransitionAudit validates the transition and, regardless of outcome,
// reports a SkillTransitionRecord to sink (nil-safe). 接线方（Batch B
// usecase 入口）据此把 Important 级跃迁送上 BlockUpTo 通道、Informational
// 级走日志，非法迁移在观测层同样留痕（设计 §8.4①「非法迁移拒绝+事件」）。
func (sm *SkillStateMachine) TransitionAudit(from SkillState, event SkillEvent, skillID string, sink func(SkillTransitionRecord)) (SkillState, error) {
	to, err := sm.inner.Transition(from, event)
	if sink != nil {
		rec := SkillTransitionRecord{SkillID: skillID, From: from, Event: event, To: to, Allowed: err == nil}
		if err == nil {
			rec.Level = SkillTransitionEventLevel(event)
		} else {
			rec.Level = SkillEventLevelInformational
		}
		sink(rec)
	}
	return to, err
}

// ── Event grading (AS-EVT-01 / 设计 §9.3) ────────────────────────────────────

// SkillTransitionEventLevel maps a successful governance transition to its
// event grading: applied/rolled_back 为 Important，validated/approved 为
// Informational（approve 的人工决策已由 learning_loop decision records 审计，
// 此事件仅状态跃迁观测）。
func SkillTransitionEventLevel(event SkillEvent) SkillEventLevel {
	switch event {
	case SkillEventApply, SkillEventRollback:
		return SkillEventLevelImportant
	default:
		return SkillEventLevelInformational
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// ParseSkillState converts a raw string to a SkillState, reporting whether it
// is a known canonical state.
func ParseSkillState(s string) (SkillState, bool) {
	switch SkillState(s) {
	case SkillStateDraft, SkillStateValidated, SkillStateApproved, SkillStateApplied, SkillStateRolledBack:
		return SkillState(s), true
	default:
		return "", false
	}
}

// IsSkillTerminalState returns true for states with no outgoing transitions.
func IsSkillTerminalState(state SkillState) bool {
	return state == SkillStateRolledBack
}

// MapSkillStateToLegacy derives the legacy dual fields (platform_skill.status,
// platform_skill.lifecycle_status) from the canonical governance state.
//
// 映射表（评审注 ①「双字段映射表灭绝歧义」）：
//
//	draft       → status=draft,     lifecycle_status=draft
//	validated   → status=draft,     lifecycle_status=validated
//	approved    → status=draft,     lifecycle_status=approved   （未生效不发布）
//	applied     → status=published, lifecycle_status=applied
//	rolled_back → status=archived,  lifecycle_status=rolled_back
//
// fail-closed：未知 canonical 状态返回 ok=false，不输出任何映射。
// 注意 status 历史别名 "active" 视为 published（skill.isPublishedStatus）。
func MapSkillStateToLegacy(state SkillState) (status string, lifecycle string, ok bool) {
	switch state {
	case SkillStateDraft:
		return "draft", string(SkillStateDraft), true
	case SkillStateValidated:
		return "draft", string(SkillStateValidated), true
	case SkillStateApproved:
		return "draft", string(SkillStateApproved), true
	case SkillStateApplied:
		return "published", string(SkillStateApplied), true
	case SkillStateRolledBack:
		return "archived", string(SkillStateRolledBack), true
	default:
		return "", "", false
	}
}
