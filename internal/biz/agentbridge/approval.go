package agentbridge

import (
	"strings"

	"aranea-agents/pkg/apierror"
)

// Confirm decisions mapped from the companion / ConfirmActivity tokens.
const (
	DecisionApprove = "approve"
	DecisionDeny    = "deny"
	DecisionAlways  = "always"
)

// ToolExternalCoding is the confirm-step tool name / source for M2 审批中继.
const ToolExternalCoding = "external_coding"

// ResolvePermissionOption maps a user decision onto an ACP option.
// rememberAlways is true when the caller should cache allow_always for this task
// (DecisionAlways, or DecisionApprove while alwaysCache is already set).
func ResolvePermissionOption(opts []PermissionOption, decision string, alwaysCache bool) (optionID string, rememberAlways bool, err error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if len(opts) == 0 {
		return "", false, apierror.BadRequest(apierror.DomainAgentBridge, "permission request without options")
	}
	switch decision {
	case DecisionAlways:
		if id := firstOptionKind(opts, "allow_always"); id != "" {
			return id, true, nil
		}
		if id := firstOptionKind(opts, "allow_once"); id != "" {
			return id, true, nil
		}
	case DecisionApprove:
		if alwaysCache {
			if id := firstOptionKind(opts, "allow_always"); id != "" {
				return id, true, nil
			}
		}
		if id := firstOptionKind(opts, "allow_once"); id != "" {
			return id, alwaysCache, nil
		}
		if id := firstOptionKind(opts, "allow_always"); id != "" {
			return id, true, nil
		}
	case DecisionDeny:
		if id := firstOptionKind(opts, "reject_once"); id != "" {
			return id, false, nil
		}
		if id := firstOptionKind(opts, "reject_always"); id != "" {
			return id, false, nil
		}
	default:
		return "", false, apierror.BadRequest(apierror.DomainAgentBridge, "unknown approval decision %s", decision)
	}
	return "", false, apierror.BadRequest(apierror.DomainAgentBridge, "no matching permission option for %s", decision)
}

func firstOptionKind(opts []PermissionOption, kind string) string {
	for _, o := range opts {
		if strings.EqualFold(strings.TrimSpace(o.Kind), kind) && strings.TrimSpace(o.OptionID) != "" {
			return o.OptionID
		}
	}
	return ""
}
