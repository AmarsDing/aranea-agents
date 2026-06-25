package agent

import (
	"context"
)

// ActivityMetaResolver supplies catalog display names and team member labels without importing biz/data.
type ActivityMetaResolver interface {
	ResolveDisplayLabel(ctx context.Context, toolName string) string
	ResolveAgentDisplayName(ctx context.Context, agentKey string) string
	ResolveAgentID(ctx context.Context, agentKey string) string
}
