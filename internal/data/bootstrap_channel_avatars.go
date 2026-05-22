package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
)

func ensureChannelPlatformAvatars(ctx context.Context, entClient *ent.Client) error {
	if entClient == nil {
		return nil
	}
	repo := &avatarRepo{data: &Data{entClient: entClient}}
	return biz.EnsureChannelPlatformAvatars(ctx, repo)
}
