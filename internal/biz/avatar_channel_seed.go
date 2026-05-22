package biz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aranea-agents/internal/biz/channelicons"
)

// EnsureChannelPlatformAvatars upserts built-in channel platform icons from embedded PNGs.
func EnsureChannelPlatformAvatars(ctx context.Context, repo AvatarRepo) error {
	if repo == nil {
		return nil
	}
	for _, spec := range ChannelPlatformAvatarSpecs() {
		if err := ensureOneChannelPlatformAvatar(ctx, repo, spec); err != nil {
			return fmt.Errorf("channel avatar %s: %w", spec.AssetKey, err)
		}
	}
	return nil
}

func ensureOneChannelPlatformAvatar(ctx context.Context, repo AvatarRepo, spec ChannelPlatformAvatarSpec) error {
	pngData, err := channelicons.LoadPNG(spec.AssetKey)
	if err != nil {
		pngData, err = RenderChannelPlatformAvatarPNG(spec)
		if err != nil {
			return err
		}
	}
	main, thumb, w, h, mime, err := processAvatarUpload(pngData, "image/png")
	if err != nil {
		return err
	}

	existing, err := repo.GetAvatarAssetByKey(ctx, spec.AssetKey)
	if err == nil && existing.ID != "" {
		return repo.UpdateAvatarAssetImages(ctx, existing.ID, main, thumb, mime, w, h, len(main))
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	asset := AvatarAsset{
		ID:            spec.AssetKey,
		Key:           spec.AssetKey,
		Name:          spec.Name,
		Description:   fmt.Sprintf("Channel platform icon (%s)", spec.ChannelType),
		MimeType:      mime,
		Source:        "system",
		IsSystem:      true,
		FileSizeBytes: len(main),
		WidthPx:       w,
		HeightPx:      h,
		SortOrder:     spec.SortOrder,
	}
	_, err = repo.CreateAvatarAsset(ctx, asset, main, thumb)
	return err
}
