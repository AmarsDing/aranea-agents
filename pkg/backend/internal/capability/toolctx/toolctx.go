package toolctx

import (
	"context"
)

type StateStore interface {
	Get(key string) (any, error)
	Set(key string, value any) error
}

type ToolContext struct {
	context.Context

	SessionID      string
	MessageID      string
	UserID         string
	AgentID        string
	AgentKey       string
	AgentName      string
	AgentIcon      string
	TraceID        string
	ApprovalID     string
	FunctionCallID string
	StateStore     StateStore
}

func New(ctx context.Context) *ToolContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ToolContext{Context: ctx}
}

func (c *ToolContext) Clone(ctx context.Context) *ToolContext {
	if c == nil {
		return New(ctx)
	}
	if ctx == nil {
		ctx = c.Context
	}
	clone := *c
	if ctx == nil {
		ctx = context.Background()
	}
	clone.Context = ctx
	return &clone
}
