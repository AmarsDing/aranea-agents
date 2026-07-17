package biz

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

type avatarSeedRepo struct {
	byKey map[string]AvatarAsset
}

func (s *avatarSeedRepo) ListAvatarAssets(context.Context, string, string, string) ([]AvatarAsset, error) {
	return nil, nil
}

func (s *avatarSeedRepo) GetAvatarAssetByKey(_ context.Context, assetKey string) (AvatarAsset, error) {
	if a, ok := s.byKey[assetKey]; ok {
		return a, nil
	}
	return AvatarAsset{}, shared.ErrNotFound
}

func (s *avatarSeedRepo) GetAvatarImage(context.Context, string, bool) (AvatarImage, error) {
	return AvatarImage{}, nil
}

func (s *avatarSeedRepo) CreateAvatarAsset(_ context.Context, asset AvatarAsset, imageData, thumbnailData []byte) (AvatarAsset, error) {
	if s.byKey == nil {
		s.byKey = map[string]AvatarAsset{}
	}
	s.byKey[asset.Key] = asset
	return asset, nil
}

func (s *avatarSeedRepo) UpdateAvatarAssetImages(_ context.Context, id string, _, _ []byte, _ string, _, _, _ int) error {
	for k, a := range s.byKey {
		if a.ID == id {
			s.byKey[k] = a
			return nil
		}
	}
	return nil
}

func (s *avatarSeedRepo) SoftDeleteAvatarAsset(context.Context, string) error { return nil }

func TestRenderChannelPlatformAvatarPNG(t *testing.T) {
	png, err := RenderChannelPlatformAvatarPNG(ChannelPlatformAvatarSpec{
		ChannelType: "feishu",
		AssetKey:    "channel_feishu",
		Name:        "飞书",
		Label:       "飞",
		R:           51,
		G:           112,
		B:           255,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 64 {
		t.Fatalf("png too small: %d", len(png))
	}
}

func TestEnsureChannelPlatformAvatarsIdempotent(t *testing.T) {
	repo := &avatarSeedRepo{}
	if err := EnsureChannelPlatformAvatars(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	n := len(repo.byKey)
	if n != len(ChannelPlatformAvatarSpecs()) {
		t.Fatalf("created %d want %d", n, len(ChannelPlatformAvatarSpecs()))
	}
	if err := EnsureChannelPlatformAvatars(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if len(repo.byKey) != n {
		t.Fatalf("expected idempotent seed, got %d", len(repo.byKey))
	}
	if _, ok := repo.byKey["channel_feishu"]; !ok {
		t.Fatal("missing channel_feishu")
	}
}

func TestChannelTypeToAssetKeySuffix(t *testing.T) {
	if channelTypeToAssetKeySuffix("wecom-app") != "wecom_app" {
		t.Fatal("wecom-app suffix")
	}
}

// apierrorNotFoundRepo mirrors data.avatarRepo which returns apierror.NotFound
// (not shared.ErrNotFound) when an asset key is missing.
type apierrorNotFoundRepo struct {
	byKey map[string]AvatarAsset
}

func (s *apierrorNotFoundRepo) ListAvatarAssets(context.Context, string, string, string) ([]AvatarAsset, error) {
	return nil, nil
}

func (s *apierrorNotFoundRepo) GetAvatarAssetByKey(_ context.Context, assetKey string) (AvatarAsset, error) {
	if a, ok := s.byKey[assetKey]; ok {
		return a, nil
	}
	return AvatarAsset{}, apierror.NotFound(apierror.DomainData, "not found")
}

func (s *apierrorNotFoundRepo) GetAvatarImage(context.Context, string, bool) (AvatarImage, error) {
	return AvatarImage{}, nil
}

func (s *apierrorNotFoundRepo) CreateAvatarAsset(_ context.Context, asset AvatarAsset, imageData, thumbnailData []byte) (AvatarAsset, error) {
	if s.byKey == nil {
		s.byKey = map[string]AvatarAsset{}
	}
	s.byKey[asset.Key] = asset
	return asset, nil
}

func (s *apierrorNotFoundRepo) UpdateAvatarAssetImages(_ context.Context, id string, _, _ []byte, _ string, _, _, _ int) error {
	return nil
}

func (s *apierrorNotFoundRepo) SoftDeleteAvatarAsset(context.Context, string) error { return nil }

func TestEnsureChannelPlatformAvatarsWithAPIErrorNotFound(t *testing.T) {
	repo := &apierrorNotFoundRepo{}
	if err := EnsureChannelPlatformAvatars(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if len(repo.byKey) != len(ChannelPlatformAvatarSpecs()) {
		t.Fatalf("created %d want %d", len(repo.byKey), len(ChannelPlatformAvatarSpecs()))
	}
}

func TestEnsureAgentAvatarsWithAPIErrorNotFound(t *testing.T) {
	repo := &apierrorNotFoundRepo{}
	if err := EnsureAgentAvatars(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if len(repo.byKey) != len(AgentAvatarSpecs()) {
		t.Fatalf("created %d want %d", len(repo.byKey), len(AgentAvatarSpecs()))
	}
}

func TestIsAvatarAssetMissing(t *testing.T) {
	if !isAvatarAssetMissing(shared.ErrNotFound) {
		t.Fatal("shared.ErrNotFound")
	}
	if !isAvatarAssetMissing(apierror.NotFound(apierror.DomainData, "not found")) {
		t.Fatal("apierror.NotFound")
	}
	if isAvatarAssetMissing(nil) || isAvatarAssetMissing(apierror.Internal("AVATAR", "boom")) {
		t.Fatal("unexpected missing")
	}
}
