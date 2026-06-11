package biz

import (
	"context"
	"errors"
	"fmt"

	"aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/biz/channelicons"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// EnsureChannelPlatformAvatars upserts built-in channel platform icons from embedded PNGs.
func EnsureChannelPlatformAvatars(ctx context.Context, repo AvatarRepo) error {
	if repo == nil {
		return nil
	}
	for _, spec := range ChannelPlatformAvatarSpecs() {
		if err := ensureOneChannelPlatformAvatar(ctx, repo, spec); err != nil {
			return apierror.Internal("AVATAR", "channel avatar %s: %s", spec.AssetKey, err.Error())
		}
	}
	return nil
}

func ensureOneChannelPlatformAvatar(ctx context.Context, repo AvatarRepo, spec ChannelPlatformAvatarSpec) error {
	existing, err := repo.GetAvatarAssetByKey(ctx, spec.AssetKey)
	if err == nil && existing.ID != "" {
		// 头像已存在且元数据一致，跳过图像处理和 UPDATE
		if existing.WidthPx == avatar.AvatarMainMaxPx && existing.FileSizeBytes > 0 {
			return nil
		}
		// 元数据不一致，需要重新处理图像
		pngData, err := channelicons.LoadPNG(spec.AssetKey)
		if err != nil {
			pngData, err = RenderChannelPlatformAvatarPNG(spec)
			if err != nil {
				return err
			}
		}
		main, thumb, w, h, mime, err := avatar.ProcessAvatarUpload(pngData, "image/png")
		if err != nil {
			return err
		}
		return repo.UpdateAvatarAssetImages(ctx, existing.ID, main, thumb, mime, w, h, len(main))
	}
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return err
	}

	// 不存在，创建新头像
	pngData, err := channelicons.LoadPNG(spec.AssetKey)
	if err != nil {
		pngData, err = RenderChannelPlatformAvatarPNG(spec)
		if err != nil {
			return err
		}
	}
	main, thumb, w, h, mime, err := avatar.ProcessAvatarUpload(pngData, "image/png")
	if err != nil {
		return err
	}

	asset := AvatarAsset{
		ID:            spec.AssetKey,
		Key:           spec.AssetKey,
		Name:          spec.Name,
		Description:   fmt.Sprintf("Channel platform icon (%s)", spec.ChannelType),
		MimeType:      mime,
		Source:        "system",
		Category:      "channel",
		IsSystem:      true,
		FileSizeBytes: len(main),
		WidthPx:       w,
		HeightPx:      h,
		SortOrder:     spec.SortOrder,
	}
	_, err = repo.CreateAvatarAsset(ctx, asset, main, thumb)
	return err
}
