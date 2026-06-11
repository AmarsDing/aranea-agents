package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools/security"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type commandSafetyBeforeHook struct {
	policy *security.CommandSafetyPolicy
	lg     loggateway.Logger
}

func newCommandSafetyBeforeHook(lg loggateway.Logger) *commandSafetyBeforeHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &commandSafetyBeforeHook{
		policy: security.NewCommandSafetyPolicy(lg),
		lg:     lg,
	}
}

func (h *commandSafetyBeforeHook) Point() callbacks.CallbackPoint { return callbacks.PointBeforeTool }
// Priority 4 executes before circuit breaker (priority 5), ensuring security
// checks run first: a blocked tool should never reach the circuit breaker.
func (h *commandSafetyBeforeHook) Priority() int                   { return 4 }

func (h *commandSafetyBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{}, nil
	}
	violation := h.policy.Evaluate(args.ToolName, args.Arguments)
	if violation != nil {
		h.lg.Warn("command safety policy blocked tool call",
			loggateway.StepID("tool.safety.policy_violation"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("rule", violation.Rule),
			loggateway.Str("path", violation.Path),
			loggateway.Str("severity", "high"),
		)
		return &trpctool.BeforeToolResult{
			CustomResult: fmt.Sprintf("Tool call blocked by security policy: %s. Access to sensitive paths (%s) is not allowed.", violation.Rule, violation.Path),
		}, nil
	}
	return &trpctool.BeforeToolResult{}, nil
}
