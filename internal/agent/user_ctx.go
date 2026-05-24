package agent

import (
	"context"

	"aranea-agents/pkg/ctxuser"
)

func WithUserID(ctx context.Context, userID string) context.Context {
	return ctxuser.WithUserID(ctx, userID)
}

func UserIDFromCtx(ctx context.Context) string {
	return ctxuser.TRPCUserKey(ctx)
}
