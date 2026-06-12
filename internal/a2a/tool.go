// Package a2a provides the call_agent trpc tool enabling Agent-to-Agent invocation.
package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
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

// InvokerFunc executes an agent capability and returns its result JSON string.
type InvokerFunc func(ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error)

// invokerKey stores the InvokerFunc in context.
type invokerKey struct{}

// WithInvoker attaches the invoker function to ctx.
func WithInvoker(ctx context.Context, fn InvokerFunc) context.Context {
	return context.WithValue(ctx, invokerKey{}, fn)
}

func invokerFromContext(ctx context.Context) InvokerFunc {
	fn, _ := ctx.Value(invokerKey{}).(InvokerFunc)
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
		Description: "Invoke a named capability on another A2A-enabled agent in the same workspace. The target agent must have A2A Endpoint enabled with the requested capability.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Agent-to-agent invocation request",
			Required:    []string{"agent_id", "capability"},
			Properties: map[string]*trpctool.Schema{
				"agent_id": {
					Type:        "string",
					Description: "Target agent ID (from A2A Discover or agent catalog)",
				},
				"capability": {
					Type:        "string",
					Description: "Capability name advertised on the target AgentCard (e.g. chat)",
				},
				"payload": {
					Description: "Optional JSON payload forwarded to the target agent (e.g. {\"message\":\"...\"})",
				},
				"timeout_seconds": {
					Type:        "integer",
					Description: "Timeout in seconds (default 30)",
				},
			},
		},
	}
}

func (t *callAgentTool) Call(ctx context.Context, args []byte) (any, error) {
	var in callAgentInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("call_agent: invalid args: %w", err)
	}
	if in.AgentID == "" {
		return nil, apierror.BadRequest(apierror.DomainA2A, "call_agent: agent_id is required")
	}
	if in.Capability == "" {
		return nil, apierror.BadRequest(apierror.DomainA2A, "call_agent: capability is required")
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

	if invoker == nil {
		return nil, apierror.Internal(apierror.DomainA2A, "call_agent: invoker not configured")
	}

	// Apply timeout as context deadline so both local and remote invocations respect it.
	invCtx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := invoker(invCtx, in.AgentID, in.Capability, payloadJSON, in.TimeoutSeconds)
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
