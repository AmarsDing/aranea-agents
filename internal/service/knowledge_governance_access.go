package service

import (
	"context"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

func assertKnowledgeGovernanceAdmin(ctx context.Context) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return auth.ErrForbidden
	}
	return nil
}
