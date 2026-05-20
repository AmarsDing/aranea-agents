package agent

import (
	"context"

	"aranea-agents/internal/event"
)

// ActivityMetaResolver supplies catalog display names and team member labels without importing biz/data.
type ActivityMetaResolver interface {
	ResolveDisplayLabel(ctx context.Context, toolName string) string
	ResolveAgentDisplayName(ctx context.Context, agentKey string) string
	ResolveAgentID(ctx context.Context, agentKey string) string
}

// ActivityPersister upserts chat.activity/v1 rows during a turn (service implements via SessionUsecase).
type ActivityPersister interface {
	UpsertActivity(ctx context.Context, meta ProjectMeta, tc event.EnvelopeToolCall) error
}
