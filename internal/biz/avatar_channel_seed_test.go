package biz

import (
	"context"
	"database/sql"
	"testing"
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
	return AvatarAsset{}, sql.ErrNoRows
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
