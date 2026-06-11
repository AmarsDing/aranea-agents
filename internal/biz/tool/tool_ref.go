package tool

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ResolveToolKey maps a catalog id or tool_key to the canonical tool_key.
func (u *ToolUsecase) ResolveToolKey(ctx context.Context, idOrKey string) (string, error) {
	idOrKey = strings.TrimSpace(idOrKey)
	if idOrKey == "" {
		return "", apierror.BadRequest("TOOL", "tool id or key is required")
	}
	t, err := u.GetTool(ctx, idOrKey)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(t.Key)
	if key == "" {
		return "", apierror.BadRequest("TOOL", "tool key is empty")
	}
	return key, nil
}
