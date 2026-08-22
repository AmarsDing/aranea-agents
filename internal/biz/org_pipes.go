package biz

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Org pipe kinds (R11 / R19). Heartbeat is observational, never a dispatch gate.
const (
	PipeUpwardHeartbeat = "heartbeat"
	PipeUpwardException = "upward"
	PipeLateralBrief    = "brief"
	PipeDownwardGrant   = "downward"
	PipeDeptMail        = "deptmail"
	PipeUserInject      = "inject"
)

// UpwardPipeMaxRunes is the R11 ceiling for a single upward payload.
const UpwardPipeMaxRunes = 2000

// ClipUpwardPayload trims an upward heartbeat/exception to ≤2KB runes.
func ClipUpwardPayload(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) <= UpwardPipeMaxRunes {
		return s
	}
	return string([]rune(s)[:UpwardPipeMaxRunes])
}

// UpwardIsDispatchBarrier reports whether an upward phase may block DAG start.
// Heartbeats never block; dependents wait on Brief (R7), not on "already reported".
func UpwardIsDispatchBarrier(phase string) bool {
	switch strings.TrimSpace(phase) {
	case PipeUpwardHeartbeat, PipeUpwardException:
		return false
	default:
		return false
	}
}

// ConfirmKind is one of the R18 five confirmation tiers.
type ConfirmKind string

const (
	ConfirmNone          ConfirmKind = ""
	ConfirmCreateAgent   ConfirmKind = "create_agent"
	ConfirmNewPlaybook   ConfirmKind = "new_playbook"
	ConfirmHighRiskGate  ConfirmKind = "high_risk_gate"
	ConfirmDangerousTool ConfirmKind = "dangerous_tool"
	ConfirmPlaybookStage ConfirmKind = "playbook_confirm_before"
)

// ConfirmInput is the evidence for R18. Default stage handoff is Brief-only.
type ConfirmInput struct {
	CreatingAgent         bool
	AuthorizingPlaybook   bool
	HighRiskGate          bool
	DangerousTool         bool
	PlaybookConfirmBefore bool
}

// NeedsUserConfirm returns the first matching R18 tier. Empty means no card.
func NeedsUserConfirm(in ConfirmInput) ConfirmKind {
	switch {
	case in.CreatingAgent:
		return ConfirmCreateAgent
	case in.AuthorizingPlaybook:
		return ConfirmNewPlaybook
	case in.HighRiskGate:
		return ConfirmHighRiskGate
	case in.DangerousTool:
		return ConfirmDangerousTool
	case in.PlaybookConfirmBefore:
		return ConfirmPlaybookStage
	default:
		return ConfirmNone
	}
}

// OrgMemberL3Scopes is the specialist L3 recall set: personal experience only.
// "team" must not be a lateral bus between sibling members (R15).
func OrgMemberL3Scopes() []string {
	return []string{"agent"}
}

// IsLateralBriefChannel reports whether a channel may carry inter-team conclusions.
func IsLateralBriefChannel(kind string) bool {
	return strings.TrimSpace(kind) == PipeLateralBrief
}

// NewUpwardProgressEvent builds an orchestration_progress notice for the
// upward pipe. It is never a dispatch barrier.
func NewUpwardProgressEvent(sessionID, phase, summary string, extra map[string]any) *SystemNoticeEvent {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		phase = PipeUpwardHeartbeat
	}
	clipped := ClipUpwardPayload(summary)
	meta := map[string]any{
		"phase":            phase,
		"pipe":             phase,
		"summary":          clipped,
		"dispatch_barrier": false,
	}
	for k, v := range extra {
		if k == "" || v == nil {
			continue
		}
		meta[k] = v
	}
	return NewSystemNoticeEvent(sessionID, "orchestration_progress", clipped, meta)
}

// PublishUpwardProgress publishes a clipped upward notice. Safe with a nil bus.
func PublishUpwardProgress(bus EventBus, ctx context.Context, sessionID, phase, summary string, extra map[string]any) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if UpwardIsDispatchBarrier(phase) {
		return
	}
	bus.Publish(ctx, NewUpwardProgressEvent(sessionID, phase, summary, extra))
}

// ClampSpecialistL3Scopes drops team/user/workspace L3 scopes for non-Spirit
// agents so sibling members cannot use L3 as a lateral bus (R15).
func ClampSpecialistL3Scopes(p *MemoryRuntimePolicy, a Agent) {
	if p == nil {
		return
	}
	if strings.TrimSpace(a.AgentKey) == SpiritAgentKey {
		return
	}
	p.L3RecallScopes = OrgMemberL3Scopes()
}
