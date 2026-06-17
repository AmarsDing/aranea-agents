package a2a

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
		return apierror.Forbidden(apierror.DomainA2A, "cross-workspace invocation is not allowed")
	}
	return nil
}

// NewInvoker returns an invokerFunc wired to exec with workspace and capability checks.
func NewInvoker(exec AgentTurnRunner, uc *biz.A2AUsecase, agents biz.AgentRepository, lg loggateway.Logger, retryPolicy a2abiz.RetryPolicy) InvokerFunc {
	return func(ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int) (string, error) {
		calleeAgentID = strings.TrimSpace(calleeAgentID)
		capability = strings.TrimSpace(capability)
		if calleeAgentID == "" {
			return "", apierror.BadRequest(apierror.DomainA2A, "callee agent_id is required")
		}
		if capability == "" {
			return "", apierror.BadRequest(apierror.DomainA2A, "capability is required")
		}

		lg.Info("A2A invoke started", loggateway.StepID("a2a.invoke"), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Str("capability", capability))

		if callerID := callerAgentIDFromContext(ctx); callerID != "" {
			callerWS := agentWorkspace(ctx, uc, agents, callerID)
			calleeWS := resolveCalleeWorkspace(ctx, uc, agents, calleeAgentID)
			if err := ValidateSameWorkspace(callerWS, calleeWS); err != nil {
				lg.Warn("A2A workspace validation failed", loggateway.StepID("a2a.invoke.workspace_fail"), loggateway.Str("caller_agent_id", callerID), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Err(err))
				return "", err
			}
		}

		target, err := ResolveInvokeTarget(ctx, uc, calleeAgentID)
		if err != nil {
			lg.Warn("A2A resolve invoke target failed", loggateway.StepID("a2a.invoke.resolve_fail"), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Err(err))
			return "", err
		}
		switch target.Kind {
		case InvokeTargetLocal:
			if err := CheckCalleeCard(target.Local, nil, capability); err != nil {
				lg.Warn("A2A callee card check failed", loggateway.StepID("a2a.invoke.card_check_fail"), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Str("capability", capability), loggateway.Err(err))
				return "", err
			}
			return invokeLocal(exec, ctx, calleeAgentID, capability, payloadJSON, timeoutSec, lg)
		case InvokeTargetRemote:
			return InvokeRemoteRegistry(ctx, target.Remote, capability, payloadJSON, timeoutSec, lg, retryPolicy)
		default:
			return "", apierror.Internal(apierror.DomainA2A, "unknown invoke target")
		}
	}
}

func invokeLocal(exec AgentTurnRunner, ctx context.Context, calleeAgentID, capability, payloadJSON string, timeoutSec int, lg loggateway.Logger) (string, error) {
	if exec == nil {
		err := apierror.Internal(apierror.DomainA2A, "agent turn runner not configured")
		lg.Warn("A2A local invoke failed", loggateway.StepID("a2a.invoke.local_fail"), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Err(err))
		return "", err
	}
	input := PayloadToInput(payloadJSON, capability)
	out, err := exec.RunAgentTurn(ctx, calleeAgentID, input, timeoutSec)
	if err != nil {
		lg.Warn("A2A local invoke failed", loggateway.StepID("a2a.invoke.local_fail"), loggateway.Str("callee_agent_id", calleeAgentID), loggateway.Err(err))
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

// InjectRunContext attaches A2A usecase, caller id, invoker, and logger to a tool run context.
func InjectRunContext(ctx context.Context, uc *biz.A2AUsecase, callerAgentID string, inv InvokerFunc, lg loggateway.Logger) context.Context {
	if uc != nil {
		ctx = WithA2AUsecase(ctx, uc)
	}
	if callerAgentID != "" {
		ctx = WithCallerAgentID(ctx, callerAgentID)
	}
	if inv != nil {
		ctx = WithInvoker(ctx, inv)
	}
	if lg != nil {
		ctx = WithLogger(ctx, lg)
	}
	return ctx
}
