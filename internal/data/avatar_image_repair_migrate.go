package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/agenticons"
	bizavatar "aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/avatarasset"
	"aranea-agents/pkg/loggateway"
)

const (
	MigrationAvatarImageRepair     = 20260729
	migrationNameAvatarImageRepair = "avatar_image_repair"
)

// RunAvatarImageRepairMigration reprocesses corrupted system avatar images.
// It targets avatars whose stored image_data does not begin with a valid JPEG
// signature. Built-in (is_system) avatars are repaired from the embedded PNG
// originals; corrupted user uploads are logged and skipped because the original
// file is not retained.
func RunAvatarImageRepairMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("avatar image repair migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationAvatarImageRepair, lg)
	if err != nil {
		return fmt.Errorf("avatar image repair migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	lg.Info("avatar image repair: starting", loggateway.StepID("migration.avatar_image_repair"))

	hasTable, err := tableExistsWithDialect(ctx, client, lg, avatarasset.Table, d)
	if err != nil {
		return fmt.Errorf("avatar image repair migration: check table: %w", err)
	}
	if !hasTable {
		if err := recordMigrationApplied(ctx, client, d, MigrationAvatarImageRepair, migrationNameAvatarImageRepair, lg); err != nil {
			return fmt.Errorf("avatar image repair migration: record: %w", err)
		}
		return nil
	}

	rows, err := client.AvatarAsset.Query().
		Where(
			avatarasset.DeletedAtEQ(""),
			avatarasset.EnabledEQ(true),
		).
		All(ctx)
	if err != nil {
		return fmt.Errorf("avatar image repair migration: query: %w", err)
	}

	var repaired, skipped, failed int
	for _, po := range rows {
		if len(po.ImageData) >= 3 && po.ImageData[0] == 0xFF && po.ImageData[1] == 0xD8 && po.ImageData[2] == 0xFF {
			skipped++
			continue
		}

		if !po.IsSystem {
			lg.Warn("avatar image repair: skipping corrupted user upload",
				loggateway.Str("asset_key", po.AssetKey),
				loggateway.Int("image_data_len", len(po.ImageData)))
			skipped++
			continue
		}

		pngData, err := agenticons.LoadPNG(po.AssetKey)
		if err != nil {
			lg.Warn("avatar image repair: cannot load embedded png",
				loggateway.Str("asset_key", po.AssetKey),
				loggateway.Err(err))
			failed++
			continue
		}

		main, thumb, w, h, mime, procErr := bizavatar.ProcessAvatarUpload(pngData, "image/png")
		if procErr != nil {
			lg.Warn("avatar image repair: reprocess failed",
				loggateway.Str("asset_key", po.AssetKey),
				loggateway.Err(procErr))
			failed++
			continue
		}

		if _, err := client.AvatarAsset.UpdateOneID(po.ID).
			SetMimeType(mime).
			SetFileSizeBytes(len(main)).
			SetWidthPx(w).
			SetHeightPx(h).
			SetImageData(main).
			SetThumbnailData(thumb).
			Save(ctx); err != nil {
			lg.Warn("avatar image repair: update failed",
				loggateway.Str("asset_key", po.AssetKey),
				loggateway.Err(err))
			failed++
			continue
		}
		repaired++
	}

	lg.Info("avatar image repair: done",
		loggateway.StepID("migration.avatar_image_repair"),
		loggateway.Int("repaired", repaired),
		loggateway.Int("skipped", skipped),
		loggateway.Int("failed", failed))

	if err := recordMigrationApplied(ctx, client, d, MigrationAvatarImageRepair, migrationNameAvatarImageRepair, lg); err != nil {
		return fmt.Errorf("avatar image repair migration: record: %w", err)
	}
	return nil
}
