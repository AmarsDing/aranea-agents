// Package a2a provides the call_agent trpc tool enabling Agent-to-Agent invocation.
package a2a

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// contextKey is used to store the A2AUsecase in context.
type contextKey struct{}

// WithA2AUsecase attaches the A2AUsecase to ctx.
func WithA2AUsecase(ctx context.Context, uc *biz.A2AUsecase) context.Context {
	return context.WithValue(ctx, contextKey{}, uc)
}

// A2AUsecaseFromContext retrieves the A2AUsecase from ctx.
func A2AUsecaseFromContext(ctx context.Context) *biz.A2AUsecase {
	uc, _ := ctx.Value(contextKey{}).(*biz.A2AUsecase)
	return uc
}

// callerIDKey is used to store the calling agent ID in context.
type callerIDKey struct{}

// WithCallerAgentID sets the caller agent identifier in context.
func WithCallerAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, callerIDKey{}, agentID)
}

func callerAgentIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(callerIDKey{}).(string)
	return id
}

// invokerFunc is a function that executes an agent capability and returns its result.
// This allows the tool to remain decoupled from the full agent runtime.
type invokerFunc func(ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error)

// invokerKey stores the invokerFunc in context.
type invokerKey struct{}

// WithInvoker attaches the invoker function to ctx.
func WithInvoker(ctx context.Context, fn invokerFunc) context.Context {
	return context.WithValue(ctx, invokerKey{}, fn)
}

func invokerFromContext(ctx context.Context) invokerFunc {
	fn, _ := ctx.Value(invokerKey{}).(invokerFunc)
	return fn
}

// callAgentInput is the JSON schema for call_agent.
type callAgentInput struct {
	AgentID        string `json:"agent_id"`
	Capability     string `json:"capability"`
	Payload        any    `json:"payload"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// NewCallAgentTool returns the call_agent tool.
func NewCallAgentTool() trpctool.CallableTool {
	return &callAgentTool{}
}

type callAgentTool struct{}

func (t *callAgentTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "call_agent",
		Description: "Invoke a named capability on another A2A-enabled agent in the same workspace.",
	}
}

func (t *callAgentTool) Call(ctx context.Context, args []byte) (any, error) {
	var in callAgentInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("call_agent: invalid args: %w", err)
	}
	if in.AgentID == "" {
		return nil, fmt.Errorf("call_agent: agent_id is required")
	}
	if in.Capability == "" {
		return nil, fmt.Errorf("call_agent: capability is required")
	}
	if in.TimeoutSeconds <= 0 {
		in.TimeoutSeconds = 30
	}

	payloadJSON := "{}"
	if in.Payload != nil {
		b, err := json.Marshal(in.Payload)
		if err != nil {
			return nil, fmt.Errorf("call_agent: payload marshal: %w", err)
		}
		payloadJSON = string(b)
	}

	uc := A2AUsecaseFromContext(ctx)
	invoker := invokerFromContext(ctx)

	// Verify that the target agent has A2A enabled before dispatching.
	if uc != nil {
		card, err := uc.GetAgentCard(ctx, in.AgentID)
		if err != nil || !card.Enabled {
			return nil, fmt.Errorf("call_agent: agent %q is not A2A-enabled", in.AgentID)
		}
		// Check that the requested capability is advertised.
		found := false
		for _, c := range card.Capabilities {
			if c.Name == in.Capability {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("call_agent: agent %q does not expose capability %q", in.AgentID, in.Capability)
		}
	}

	if invoker == nil {
		return nil, fmt.Errorf("call_agent: invoker not configured")
	}

	result, err := invoker(ctx, in.AgentID, in.Capability, payloadJSON, in.TimeoutSeconds)
	if err != nil {
		// Audit the failure when the usecase is available.
		if uc != nil {
			_ = uc.AppendAudit(ctx, biz.A2AAuditEntry{
				CallerAgentID: callerAgentIDFromContext(ctx),
				CalleeAgentID: in.AgentID,
				Capability:    in.Capability,
				Status:        "error",
			})
		}
		return nil, fmt.Errorf("call_agent: %w", err)
	}

	if uc != nil {
		_ = uc.AppendAudit(ctx, biz.A2AAuditEntry{
			CallerAgentID: callerAgentIDFromContext(ctx),
			CalleeAgentID: in.AgentID,
			Capability:    in.Capability,
			Status:        "success",
		})
	}

	return map[string]any{"result": result}, nil
}
