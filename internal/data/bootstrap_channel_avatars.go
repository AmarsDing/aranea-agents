package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureChannelPlatformAvatars(ctx context.Context, entClient *ent.Client, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	repo := &avatarRepo{data: &Data{entClient: entClient}}
	if err := biz.EnsureChannelPlatformAvatars(ctx, repo); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.channel_avatars"), loggateway.Err(err))
		return err
	}
	return nil
}
