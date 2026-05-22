package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

// AvatarAsset is a catalog row without raw image payload (list API).
type AvatarAsset struct {
	ID            string
	Key           string
	Name          string
	Description   string
	MimeType      string
	WorkspaceID   string
	OwnerUserID   string
	Source        string
	IsSystem      bool
	FileSizeBytes int
	WidthPx       int
	HeightPx      int
	SortOrder     int
	CreatedAt     string
}

// AvatarImage holds binary payload for GET file/thumbnail endpoints.
type AvatarImage struct {
	ID       string
	MimeType string
	Data     []byte
}

// AvatarRepo abstracts avatar_assets persistence (legacy SQLite layout).
type AvatarRepo interface {
	ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]AvatarAsset, error)
	GetAvatarAssetByKey(ctx context.Context, assetKey string) (AvatarAsset, error)
	GetAvatarImage(ctx context.Context, id string, thumbnail bool) (AvatarImage, error)
	CreateAvatarAsset(ctx context.Context, asset AvatarAsset, imageData, thumbnailData []byte) (AvatarAsset, error)
	UpdateAvatarAssetImages(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error
	SoftDeleteAvatarAsset(ctx context.Context, id string) error
}

// AvatarUsecase implements avatar workflows.
type AvatarUsecase struct {
	repo AvatarRepo
}

func NewAvatarUsecase(repo AvatarRepo) *AvatarUsecase {
	return &AvatarUsecase{repo: repo}
}

func (uc *AvatarUsecase) ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]AvatarAsset, error) {
	return uc.repo.ListAvatarAssets(ctx, strings.TrimSpace(scope), strings.TrimSpace(workspaceID), strings.TrimSpace(ownerUserID))
}

func (uc *AvatarUsecase) GetAvatarImage(ctx context.Context, id string, thumbnail bool) (AvatarImage, error) {
	img, err := uc.repo.GetAvatarImage(ctx, id, thumbnail)
	if err != nil {
		return AvatarImage{}, err
	}
	return img, nil
}

var fallbackAvatarID atomic.Uint64

func newAvatarID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := fallbackAvatarID.Add(1)
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405"))) + hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

func (uc *AvatarUsecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (AvatarAsset, error) {
	if len(data) == 0 {
		return AvatarAsset{}, errors.BadRequest("AVATAR", "avatar file is required")
	}
	const max = 2 * 1024 * 1024
	if len(data) > max {
		return AvatarAsset{}, errors.BadRequest("AVATAR", "avatar file must be <= 2MB")
	}
	mt := http.DetectContentType(data)
	if mt != "image/png" && mt != "image/jpeg" && mt != "image/webp" && mt != "image/gif" {
		return AvatarAsset{}, errors.BadRequest("AVATAR", "unsupported avatar type")
	}
	mainData, thumbData, width, height, outMime, procErr := processAvatarUpload(data, mt)
	if procErr != nil {
		return AvatarAsset{}, procErr
	}
	id := newAvatarID()
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "上传头像"
	}
	asset := AvatarAsset{
		ID:            id,
		Key:           "upload-" + id,
		Name:          name,
		Description:   "用户上传头像",
		MimeType:      outMime,
		WorkspaceID:   workspaceID,
		OwnerUserID:   ownerUserID,
		Source:        "upload",
		IsSystem:      false,
		FileSizeBytes: len(mainData),
		WidthPx:       width,
		HeightPx:      height,
		SortOrder:     1000,
	}
	return uc.repo.CreateAvatarAsset(ctx, asset, mainData, thumbData)
}

func (uc *AvatarUsecase) DeleteAvatarAsset(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("AVATAR", "avatar id is required")
	}
	return uc.repo.SoftDeleteAvatarAsset(ctx, id)
}
