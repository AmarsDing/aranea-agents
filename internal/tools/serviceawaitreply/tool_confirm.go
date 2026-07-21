package serviceawaitreply

import (
	"context"
	"strings"
)

type toolConfirmRequestKey struct{}

// ToolConfirmRequest describes a mid-turn tool approval prompt.
type ToolConfirmRequest struct {
	ToolKey    string
	ToolCallID string
}

const (
	// ReplyApprove is the structured approve token for tool confirmation UI.
	ReplyApprove = "__aranea:tool_confirm:approve"
	// ReplyDeny is the structured deny token for tool confirmation UI.
	ReplyDeny = "__aranea:tool_confirm:deny"
	// ReplyApproveSession approves the tool and grants it for the rest of
	// the current session (session-scoped grant, in-memory only).
	ReplyApproveSession = "__aranea:tool_confirm:approve_session"
	// ReplyApproveAlways approves the tool and grants it persistently
	// across sessions (persisted grant, stored in DB).
	ReplyApproveAlways = "__aranea:tool_confirm:approve_always"
)

// ToolConfirmOutcome is the parsed result of a structured tool confirmation
// reply from the UI.
type ToolConfirmOutcome int

const (
	// ToolConfirmOutcomeDeny rejects the tool invocation.
	ToolConfirmOutcomeDeny ToolConfirmOutcome = iota
	// ToolConfirmOutcomeApprove allows this invocation only.
	ToolConfirmOutcomeApprove
	// ToolConfirmOutcomeApproveSession allows this invocation and grants the
	// tool for the remainder of the current session.
	ToolConfirmOutcomeApproveSession
	// ToolConfirmOutcomeApproveAlways allows this invocation and persists a
	// grant so future sessions skip confirmation for the tool.
	ToolConfirmOutcomeApproveAlways
)

// Approved reports whether the outcome allows the tool to run.
func (o ToolConfirmOutcome) Approved() bool {
	return o != ToolConfirmOutcomeDeny
}

// WithToolConfirmRequest attaches tool confirmation metadata to ctx for ReplyFunc handlers.
func WithToolConfirmRequest(ctx context.Context, req ToolConfirmRequest) context.Context {
	return context.WithValue(ctx, toolConfirmRequestKey{}, req)
}

// ToolConfirmRequestFromContext returns tool confirmation metadata when present.
func ToolConfirmRequestFromContext(ctx context.Context) (ToolConfirmRequest, bool) {
	req, ok := ctx.Value(toolConfirmRequestKey{}).(ToolConfirmRequest)
	if !ok {
		return ToolConfirmRequest{}, false
	}
	req.ToolKey = strings.TrimSpace(req.ToolKey)
	req.ToolCallID = strings.TrimSpace(req.ToolCallID)
	if req.ToolKey == "" {
		return ToolConfirmRequest{}, false
	}
	return req, true
}

// ParseToolConfirmReply interprets structured approve/deny replies from the UI.
func ParseToolConfirmReply(reply string) (approved bool, structured bool) {
	outcome, structured := ParseToolConfirmOutcome(reply)
	return outcome.Approved(), structured
}

// ParseToolConfirmOutcome interprets structured tool confirmation replies,
// including session-scoped and persistent grant variants.
func ParseToolConfirmOutcome(reply string) (ToolConfirmOutcome, bool) {
	switch strings.TrimSpace(reply) {
	case ReplyApprove:
		return ToolConfirmOutcomeApprove, true
	case ReplyDeny:
		return ToolConfirmOutcomeDeny, true
	case ReplyApproveSession:
		return ToolConfirmOutcomeApproveSession, true
	case ReplyApproveAlways:
		return ToolConfirmOutcomeApproveAlways, true
	default:
		return ToolConfirmOutcomeDeny, false
	}
}
