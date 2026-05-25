package service

import (
	"context"

	"aranea-agents/internal/biz"
	sessctx "aranea-agents/internal/session"
)

func (o *ChatOrchestrator) resolveContextWindowTokens(ctx context.Context, sess biz.Session, ag biz.Agent, prov, mod string) int {
	return sessctx.ResolveContextWindowTokens(ctx, o.llmContextCatalog(), sess, ag, prov, mod)
}
