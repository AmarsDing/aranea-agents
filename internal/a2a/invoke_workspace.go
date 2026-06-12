package a2a

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// ValidateAdminInvokeWorkspace enforces workspace scope for HTTP Admin Invoke.
func ValidateAdminInvokeWorkspace(ctx context.Context, reqWorkspace string, card biz.A2AAgentCard) error {
	calleeWS := strings.TrimSpace(card.Workspace)
	if calleeWS == "" {
		return nil
	}
	if reqWS := strings.TrimSpace(reqWorkspace); reqWS != "" && reqWS != calleeWS {
		return apierror.Forbidden(apierror.DomainA2A, "callee agent is not in the requested workspace")
	}
	ctxWS, ok := workspace.FromContext(ctx)
	if ok && ctxWS != "" && ctxWS != workspace.DefaultWorkspaceID && ctxWS != workspace.SystemWorkspaceID && ctxWS != calleeWS {
		return apierror.Forbidden(apierror.DomainA2A, "cross-workspace invocation is not allowed")
	}
	return nil
}
