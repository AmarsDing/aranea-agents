package a2a

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ValidateAdminInvokeWorkspace enforces workspace scope for HTTP Admin Invoke.
func ValidateAdminInvokeWorkspace(ctx context.Context, reqWorkspace string, card biz.A2AAgentCard) error {
	calleeWS := strings.TrimSpace(card.Workspace)
	if calleeWS == "" {
		return nil
	}
	if reqWS := strings.TrimSpace(reqWorkspace); reqWS != "" && reqWS != calleeWS {
		return kerrors.Forbidden("A2A", "callee agent is not in the requested workspace")
	}
	ctxWS, ok := workspace.FromContext(ctx)
	if ok && ctxWS != "" && ctxWS != workspace.DefaultWorkspaceID && ctxWS != workspace.SystemWorkspaceID && ctxWS != calleeWS {
		return kerrors.Forbidden("A2A", "cross-workspace invocation is not allowed")
	}
	return nil
}
