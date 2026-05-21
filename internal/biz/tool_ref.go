package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// ResolveToolKey maps a catalog id or tool_key to the canonical tool_key.
func (u *ToolUsecase) ResolveToolKey(ctx context.Context, idOrKey string) (string, error) {
	idOrKey = strings.TrimSpace(idOrKey)
	if idOrKey == "" {
		return "", errors.BadRequest("TOOL", "tool id or key is required")
	}
	t, err := u.GetTool(ctx, idOrKey)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(t.Key)
	if key == "" {
		return "", errors.BadRequest("TOOL", "tool key is empty")
	}
	return key, nil
}
