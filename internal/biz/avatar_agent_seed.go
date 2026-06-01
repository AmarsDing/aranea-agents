package biz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz/agenticons"
	"aranea-agents/internal/biz/avatar"
)

func EnsureAgentAvatars(ctx context.Context, repo AvatarRepo) error {
	if repo == nil {
		return nil
	}
	for _, spec := range AgentAvatarSpecs() {
		if err := ensureOneAgentAvatar(ctx, repo, spec); err != nil {
			return kerrors.InternalServer("AVATAR", fmt.Sprintf("agent avatar %s: %s", spec.AssetKey, err.Error()))
		}
	}
	return nil
}

func ensureOneAgentAvatar(ctx context.Context, repo AvatarRepo, spec AgentAvatarSpec) error {
	pngData, err := agenticons.LoadPNG(spec.AssetKey)
	if err != nil {
		return fmt.Errorf("load embedded png %s: %w", spec.AssetKey, err)
	}
	main, thumb, w, h, mime, procErr := avatar.ProcessAvatarUpload(pngData, "image/png")
	if procErr != nil {
		return procErr
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
		Description:   "Built-in agent avatar",
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
