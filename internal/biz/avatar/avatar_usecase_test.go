package avatar_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"testing"

	"aranea-agents/internal/biz/avatar"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type mockAvatarRepo struct {
	listAvatarAssetsFn        func(ctx context.Context, scope, workspaceID, ownerUserID string) ([]avatar.Asset, error)
	getAvatarAssetByKeyFn     func(ctx context.Context, assetKey string) (avatar.Asset, error)
	getAvatarImageFn          func(ctx context.Context, id string, thumbnail bool) (avatar.Image, error)
	createAvatarAssetFn       func(ctx context.Context, asset avatar.Asset, imageData, thumbnailData []byte) (avatar.Asset, error)
	updateAvatarAssetImagesFn func(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error
	softDeleteAvatarAssetFn   func(ctx context.Context, id string) error
}

type noopChannelIconRefresher struct{}

func (noopChannelIconRefresher) RefreshChannelPlatformIcons(_ context.Context, _ avatar.Repo) (*avatar.RefreshChannelPlatformIconsResult, error) {
	return &avatar.RefreshChannelPlatformIconsResult{}, nil
}

func (m *mockAvatarRepo) ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]avatar.Asset, error) {
	if m.listAvatarAssetsFn != nil {
		return m.listAvatarAssetsFn(ctx, scope, workspaceID, ownerUserID)
	}
	return nil, nil
}

func (m *mockAvatarRepo) GetAvatarAssetByKey(ctx context.Context, assetKey string) (avatar.Asset, error) {
	if m.getAvatarAssetByKeyFn != nil {
		return m.getAvatarAssetByKeyFn(ctx, assetKey)
	}
	return avatar.Asset{}, nil
}

func (m *mockAvatarRepo) GetAvatarImage(ctx context.Context, id string, thumbnail bool) (avatar.Image, error) {
	if m.getAvatarImageFn != nil {
		return m.getAvatarImageFn(ctx, id, thumbnail)
	}
	return avatar.Image{}, nil
}

func (m *mockAvatarRepo) CreateAvatarAsset(ctx context.Context, asset avatar.Asset, imageData, thumbnailData []byte) (avatar.Asset, error) {
	if m.createAvatarAssetFn != nil {
		return m.createAvatarAssetFn(ctx, asset, imageData, thumbnailData)
	}
	return asset, nil
}

func (m *mockAvatarRepo) UpdateAvatarAssetImages(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error {
	if m.updateAvatarAssetImagesFn != nil {
		return m.updateAvatarAssetImagesFn(ctx, id, imageData, thumbnailData, mime, width, height, fileSize)
	}
	return nil
}

func (m *mockAvatarRepo) SoftDeleteAvatarAsset(ctx context.Context, id string) error {
	if m.softDeleteAvatarAssetFn != nil {
		return m.softDeleteAvatarAssetFn(ctx, id)
	}
	return nil
}

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUsecase_ListAvatarAssets(t *testing.T) {
	t.Run("trims_inputs", func(t *testing.T) {
		repo := &mockAvatarRepo{
			listAvatarAssetsFn: func(_ context.Context, scope, workspaceID, ownerUserID string) ([]avatar.Asset, error) {
				if scope != "global" || workspaceID != "ws1" || ownerUserID != "u1" {
					t.Errorf("scope=%q ws=%q owner=%q", scope, workspaceID, ownerUserID)
				}
				return []avatar.Asset{{ID: "a1"}}, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		items, err := uc.ListAvatarAssets(context.Background(), "  global  ", "  ws1  ", "  u1  ")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items", len(items))
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockAvatarRepo{
			listAvatarAssetsFn: func(context.Context, string, string, string) ([]avatar.Asset, error) {
				return nil, fmt.Errorf("db fail")
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		_, err := uc.ListAvatarAssets(context.Background(), "", "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		repo := &mockAvatarRepo{
			listAvatarAssetsFn: func(context.Context, string, string, string) ([]avatar.Asset, error) {
				return nil, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		items, err := uc.ListAvatarAssets(context.Background(), "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}
	})
}

func TestUsecase_GetAvatarImage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := avatar.Image{ID: "a1", MimeType: "image/png", Data: []byte{1, 2, 3}}
		repo := &mockAvatarRepo{
			getAvatarImageFn: func(_ context.Context, id string, thumbnail bool) (avatar.Image, error) {
				if id != "a1" || !thumbnail {
					t.Errorf("id=%q thumbnail=%v", id, thumbnail)
				}
				return want, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		got, err := uc.GetAvatarImage(context.Background(), "a1", true)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "a1" {
			t.Fatalf("got ID=%q", got.ID)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockAvatarRepo{
			getAvatarImageFn: func(context.Context, string, bool) (avatar.Image, error) {
				return avatar.Image{}, fmt.Errorf("not found")
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		_, err := uc.GetAvatarImage(context.Background(), "missing", false)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsecase_UploadAvatar(t *testing.T) {
	t.Run("empty_data", func(t *testing.T) {
		uc := avatar.NewUsecase(&mockAvatarRepo{}, noopChannelIconRefresher{})
		_, err := uc.UploadAvatar(context.Background(), nil, "avatar.png", "ws1", "u1")
		if err == nil {
			t.Fatal("expected error for empty data")
		}
		ke := kerrors.FromError(err)
		if ke.Reason != "AVATAR" {
			t.Fatalf("reason=%q, want AVATAR", ke.Reason)
		}
	})

	t.Run("exceeds_2mb", func(t *testing.T) {
		uc := avatar.NewUsecase(&mockAvatarRepo{}, noopChannelIconRefresher{})
		big := make([]byte, 2*1024*1024+1)
		_, err := uc.UploadAvatar(context.Background(), big, "big.png", "ws1", "u1")
		if err == nil {
			t.Fatal("expected error for oversized upload")
		}
		ke := kerrors.FromError(err)
		if ke.Reason != "AVATAR" {
			t.Fatalf("reason=%q, want AVATAR", ke.Reason)
		}
	})

	t.Run("unsupported_type", func(t *testing.T) {
		uc := avatar.NewUsecase(&mockAvatarRepo{}, noopChannelIconRefresher{})
		pdfData := []byte("%PDF-1.4 fake pdf content that is long enough for detection")
		_, err := uc.UploadAvatar(context.Background(), pdfData, "doc.pdf", "ws1", "u1")
		if err == nil {
			t.Fatal("expected error for unsupported type")
		}
		ke := kerrors.FromError(err)
		if ke.Reason != "AVATAR" {
			t.Fatalf("reason=%q, want AVATAR", ke.Reason)
		}
	})

	t.Run("valid_png", func(t *testing.T) {
		pngData := makePNG(t, 100, 100)
		repo := &mockAvatarRepo{
			createAvatarAssetFn: func(_ context.Context, asset avatar.Asset, imageData, thumbnailData []byte) (avatar.Asset, error) {
				if asset.WorkspaceID != "ws1" || asset.OwnerUserID != "u1" {
					t.Errorf("ws=%q owner=%q", asset.WorkspaceID, asset.OwnerUserID)
				}
				if asset.MimeType != "image/jpeg" {
					t.Errorf("mime=%q, want image/jpeg", asset.MimeType)
				}
				if asset.Source != "upload" {
					t.Errorf("source=%q", asset.Source)
				}
				if asset.IsSystem {
					t.Error("IsSystem should be false")
				}
				if len(imageData) == 0 || len(thumbnailData) == 0 {
					t.Error("expected image and thumbnail data")
				}
				if asset.WidthPx <= 0 || asset.HeightPx <= 0 {
					t.Errorf("width=%d height=%d", asset.WidthPx, asset.HeightPx)
				}
				return asset, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		got, err := uc.UploadAvatar(context.Background(), pngData, "avatar.png", "ws1", "u1")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if got.Key != "upload-"+got.ID {
			t.Fatalf("key=%q", got.Key)
		}
	})

	t.Run("empty_filename_defaults", func(t *testing.T) {
		pngData := makePNG(t, 50, 50)
		repo := &mockAvatarRepo{
			createAvatarAssetFn: func(_ context.Context, asset avatar.Asset, imageData, thumbnailData []byte) (avatar.Asset, error) {
				if asset.Name != "\u4e0a\u4f20\u5934\u50cf" {
					t.Errorf("name=%q, want default", asset.Name)
				}
				return asset, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		got, err := uc.UploadAvatar(context.Background(), pngData, "", "ws1", "u1")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "\u4e0a\u4f20\u5934\u50cf" {
			t.Fatalf("name=%q", got.Name)
		}
	})

	t.Run("whitespace_filename_defaults", func(t *testing.T) {
		pngData := makePNG(t, 50, 50)
		repo := &mockAvatarRepo{
			createAvatarAssetFn: func(_ context.Context, asset avatar.Asset, imageData, thumbnailData []byte) (avatar.Asset, error) {
				if asset.Name != "\u4e0a\u4f20\u5934\u50cf" {
					t.Errorf("name=%q", asset.Name)
				}
				return asset, nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		got, err := uc.UploadAvatar(context.Background(), pngData, "   ", "ws1", "u1")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "\u4e0a\u4f20\u5934\u50cf" {
			t.Fatalf("name=%q", got.Name)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		pngData := makePNG(t, 50, 50)
		repo := &mockAvatarRepo{
			createAvatarAssetFn: func(context.Context, avatar.Asset, []byte, []byte) (avatar.Asset, error) {
				return avatar.Asset{}, fmt.Errorf("db fail")
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		_, err := uc.UploadAvatar(context.Background(), pngData, "a.png", "ws1", "u1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsecase_DeleteAvatarAsset(t *testing.T) {
	t.Run("empty_id", func(t *testing.T) {
		uc := avatar.NewUsecase(&mockAvatarRepo{}, noopChannelIconRefresher{})
		err := uc.DeleteAvatarAsset(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty id")
		}
		ke := kerrors.FromError(err)
		if ke.Reason != "AVATAR" {
			t.Fatalf("reason=%q, want AVATAR", ke.Reason)
		}
	})

	t.Run("whitespace_id", func(t *testing.T) {
		uc := avatar.NewUsecase(&mockAvatarRepo{}, noopChannelIconRefresher{})
		err := uc.DeleteAvatarAsset(context.Background(), "   ")
		if err == nil {
			t.Fatal("expected error for whitespace id")
		}
	})

	t.Run("success", func(t *testing.T) {
		called := false
		repo := &mockAvatarRepo{
			softDeleteAvatarAssetFn: func(_ context.Context, id string) error {
				called = true
				if id != "a1" {
					t.Errorf("id=%q, want a1", id)
				}
				return nil
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		if err := uc.DeleteAvatarAsset(context.Background(), "a1"); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("expected SoftDeleteAvatarAsset to be called")
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockAvatarRepo{
			softDeleteAvatarAssetFn: func(context.Context, string) error {
				return fmt.Errorf("db fail")
			},
		}
		uc := avatar.NewUsecase(repo, noopChannelIconRefresher{})
		err := uc.DeleteAvatarAsset(context.Background(), "a1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
