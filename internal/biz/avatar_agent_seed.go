package biz

import (
	"context"
	"errors"

	"aranea-agents/internal/biz/agenticons"
	"aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

func EnsureAgentAvatars(ctx context.Context, repo AvatarRepo) error {
	if repo == nil {
		return nil
	}
	for _, spec := range AgentAvatarSpecs() {
		if err := ensureOneAgentAvatar(ctx, repo, spec); err != nil {
			return apierror.Internal(apierror.DomainAvatar, "agent avatar %s: %s", spec.AssetKey, err.Error())
		}
	}
	return nil
}

func ensureOneAgentAvatar(ctx context.Context, repo AvatarRepo, spec AgentAvatarSpec) error {
	existing, err := repo.GetAvatarAssetByKey(ctx, spec.AssetKey)
	if err == nil && existing.ID != "" {
		// 头像已存在且元数据一致，跳过图像处理和 UPDATE
		if existing.WidthPx == avatar.AvatarMainMaxPx && existing.FileSizeBytes > 0 {
			return nil
		}
		// 元数据不一致，需要重新处理图像
		pngData, err := agenticons.LoadPNG(spec.AssetKey)
		if err != nil {
			return apierror.Internal(apierror.DomainAvatar, "load embedded png %s: %s", spec.AssetKey, err.Error())
		}
		main, thumb, w, h, mime, procErr := avatar.ProcessAvatarUpload(pngData, "image/png")
		if procErr != nil {
			return procErr
		}
		return repo.UpdateAvatarAssetImages(ctx, existing.ID, main, thumb, mime, w, h, len(main))
	}
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return err
	}

	// 不存在，创建新头像
	pngData, err := agenticons.LoadPNG(spec.AssetKey)
	if err != nil {
		return apierror.Internal(apierror.DomainAvatar, "load embedded png %s: %s", spec.AssetKey, err.Error())
	}
	main, thumb, w, h, mime, procErr := avatar.ProcessAvatarUpload(pngData, "image/png")
	if procErr != nil {
		return procErr
	}

	asset := AvatarAsset{
		ID:            spec.AssetKey,
		Key:           spec.AssetKey,
		Name:          spec.Name,
		Description:   "内置 Agent 头像",
		MimeType:      mime,
		Source:        "system",
		Category:      "agent",
		IsSystem:      true,
		FileSizeBytes: len(main),
		WidthPx:       w,
		HeightPx:      h,
		SortOrder:     spec.SortOrder,
	}
	_, err = repo.CreateAvatarAsset(ctx, asset, main, thumb)
	return err
}
