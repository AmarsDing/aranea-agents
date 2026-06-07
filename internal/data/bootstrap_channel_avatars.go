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
	d := &Data{entClient: entClient, readClient: entClient, rw: NewReadWriteClient(entClient, entClient), lg: lg}
	repo := &avatarRepo{data: d}
	// 用事务包裹所有头像操作，减少 SQLite WAL 锁开销
	return d.ExecInTx(ctx, func(txCtx context.Context) error {
		return biz.EnsureChannelPlatformAvatars(txCtx, repo)
	})
}
