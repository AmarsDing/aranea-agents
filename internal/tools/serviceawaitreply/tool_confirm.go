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
)

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
	reply = strings.TrimSpace(reply)
	switch reply {
	case ReplyApprove:
		return true, true
	case ReplyDeny:
		return false, true
	default:
		return false, false
	}
}
