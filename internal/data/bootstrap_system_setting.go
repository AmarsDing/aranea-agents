package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/systemsetting"
)

const systemSettingSingletonID = 1

func ensureDefaultSystemSetting(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return fmt.Errorf("ent client is nil")
	}
	exists, err := client.SystemSetting.Query().Where(
		systemsetting.IDEQ(systemSettingSingletonID),
	).Exist(ctx)
	if err != nil {
		return fmt.Errorf("system_setting probe: %w", err)
	}
	if exists {
		return nil
	}
	if err := client.SystemSetting.Create().
		SetID(systemSettingSingletonID).
		SetRootDirectory("").
		SetWorkDirectory("").
		Exec(ctx); err != nil {
		return fmt.Errorf("system_setting seed: %w", err)
	}
	return nil
}
