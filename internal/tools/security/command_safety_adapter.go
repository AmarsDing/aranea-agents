package security

import (
	"context"
	"fmt"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CommandSafetyPermissionChecker adapts the project's CommandSafetyPolicy to
// the framework's tool.PermissionChecker interface. It enables the framework's
// built-in permission check flow to enforce command safety rules before tool
// execution.
//
// Decision logic:
//   - Protected tool + protected path in args → DenyPermission
//   - All other cases → AllowPermission
type CommandSafetyPermissionChecker struct {
	policy *CommandSafetyPolicy
	lg     loggateway.Logger
}

// Compile-time interface compliance check.
var _ trpctool.PermissionChecker = (*CommandSafetyPermissionChecker)(nil)

// NewCommandSafetyPermissionChecker creates a PermissionChecker backed by the
// project's CommandSafetyPolicy.
func NewCommandSafetyPermissionChecker(lg loggateway.Logger) *CommandSafetyPermissionChecker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CommandSafetyPermissionChecker{
		policy: NewCommandSafetyPolicy(lg),
		lg:     lg,
	}
}

// NewCommandSafetyPermissionCheckerWithPolicy creates a PermissionChecker from
// an existing CommandSafetyPolicy. Returns an error if policy is nil.
func NewCommandSafetyPermissionCheckerWithPolicy(policy *CommandSafetyPolicy, lg loggateway.Logger) (*CommandSafetyPermissionChecker, error) {
	if policy == nil {
		return nil, fmt.Errorf("security: CommandSafetyPolicy must not be nil")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CommandSafetyPermissionChecker{
		policy: policy,
		lg:     lg,
	}, nil
}

// CheckPermission evaluates the tool call against the command safety policy.
// For protected tools that access sensitive paths, it returns a deny decision.
// For all other cases, it returns an allow decision.
func (c *CommandSafetyPermissionChecker) CheckPermission(
	ctx context.Context,
	req *trpctool.PermissionRequest,
) (trpctool.PermissionDecision, error) {
	if req == nil {
		return trpctool.AllowPermission(), nil
	}

	violation := c.policy.Evaluate(req.ToolName, req.Arguments)
	if violation != nil {
		c.lg.Warn("command safety permission checker denied tool call",
			loggateway.StepID("tool.safety.permission"),
			loggateway.Str("tool", req.ToolName),
			loggateway.Str("rule", violation.Rule),
			loggateway.Str("path", violation.Path),
		)
		return trpctool.DenyPermission(
			fmt.Sprintf("Access to sensitive path blocked: %s (%s)", violation.Path, violation.Rule),
		), nil
	}

	return trpctool.AllowPermission(), nil
}
