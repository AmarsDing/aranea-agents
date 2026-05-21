package a2a

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// AgentTurnRunner executes one agent turn and returns the assistant text output.
type AgentTurnRunner interface {
	RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error)
}

// PayloadToInput extracts user-visible input from an A2A payload JSON string.
func PayloadToInput(payloadJSON, capability string) string {
	payloadJSON = strings.TrimSpace(payloadJSON)
	if payloadJSON == "" || payloadJSON == "{}" {
		if capability != "" {
			return capability
		}
		return "Hello"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &m); err != nil {
		return payloadJSON
	}
	for _, key := range []string{"input", "message", "content", "text", "query", "prompt"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	if capability != "" {
		return capability + ": " + payloadJSON
	}
	return payloadJSON
}

func agentWorkspace(ctx context.Context, uc *biz.A2AUsecase, agents biz.AgentRepository, agentID string) string {
	if uc != nil {
		if card, err := uc.GetAgentCard(ctx, agentID); err == nil && strings.TrimSpace(card.Workspace) != "" {
			return strings.TrimSpace(card.Workspace)
		}
	}
	if agents != nil {
		if ag, err := agents.GetAgentByID(ctx, agentID); err == nil {
			if ag.Settings != nil {
				if s := strings.TrimSpace(ag.Settings.GetIdentity().Workspace); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// ValidateSameWorkspace rejects cross-workspace calls when both sides declare a workspace.
func ValidateSameWorkspace(callerWS, calleeWS string) error {
	callerWS = strings.TrimSpace(callerWS)
	calleeWS = strings.TrimSpace(calleeWS)
	if callerWS == "" || calleeWS == "" {
		return nil
	}
	if callerWS != calleeWS {
		return kerrors.Forbidden("A2A", "cross-workspace invocation is not allowed")
	}
	return nil
}

// NewInvoker returns an invokerFunc wired to exec with workspace and capability checks.
func NewInvoker(exec AgentTurnRunner, uc *biz.A2AUsecase, agents biz.AgentRepository) InvokerFunc {
	return func(ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error) {
		calleeAgentID = strings.TrimSpace(calleeAgentID)
		capability = strings.TrimSpace(capability)
		if calleeAgentID == "" {
			return "", kerrors.BadRequest("A2A", "callee agent_id is required")
		}
		if capability == "" {
			return "", kerrors.BadRequest("A2A", "capability is required")
		}

		if callerID := callerAgentIDFromContext(ctx); callerID != "" {
			callerWS := agentWorkspace(ctx, uc, agents, callerID)
			calleeWS := resolveCalleeWorkspace(ctx, uc, agents, calleeAgentID)
			if err := ValidateSameWorkspace(callerWS, calleeWS); err != nil {
				return "", err
			}
		}

		target, err := ResolveInvokeTarget(ctx, uc, calleeAgentID)
		if err != nil {
			return "", err
		}
		switch target.Kind {
		case InvokeTargetLocal:
			if err := CheckCalleeCard(target.Local, nil, capability); err != nil {
				return "", err
			}
			return invokeLocal(exec, ctx, calleeAgentID, capability, payloadJSON, timeoutSec)
		case InvokeTargetRemote:
			return InvokeRemoteRegistry(ctx, target.Remote, capability, payloadJSON, timeoutSec)
		default:
			return "", kerrors.InternalServer("A2A", "unknown invoke target")
		}
	}
}

func invokeLocal(exec AgentTurnRunner, ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error) {
	if exec == nil {
		return "", kerrors.InternalServer("A2A", "agent turn runner not configured")
	}
	input := PayloadToInput(payloadJSON, capability)
	out, err := exec.RunAgentTurn(ctx, calleeAgentID, input, timeoutSec)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(map[string]any{
		"capability": capability,
		"output":     out,
	})
	if err != nil {
		return out, nil
	}
	return string(result), nil
}

func resolveCalleeWorkspace(ctx context.Context, uc *biz.A2AUsecase, agents biz.AgentRepository, calleeAgentID string) string {
	if ws := agentWorkspace(ctx, uc, agents, calleeAgentID); ws != "" {
		return ws
	}
	if uc != nil {
		if remote, err := uc.GetRemoteAgent(ctx, calleeAgentID); err == nil {
			return strings.TrimSpace(remote.Workspace)
		}
	}
	return ""
}

// InjectRunContext attaches A2A usecase, caller id, and invoker to a tool run context.
func InjectRunContext(ctx context.Context, uc *biz.A2AUsecase, callerAgentID string, inv InvokerFunc) context.Context {
	if uc != nil {
		ctx = WithA2AUsecase(ctx, uc)
	}
	if callerAgentID != "" {
		ctx = WithCallerAgentID(ctx, callerAgentID)
	}
	if inv != nil {
		ctx = WithInvoker(ctx, inv)
	}
	return ctx
}
