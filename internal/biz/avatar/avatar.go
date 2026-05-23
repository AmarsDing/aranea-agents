// Package avatar implements avatar asset workflows: upload, list, delete, and image processing.
package avatar

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

// Asset is a catalog row without raw image payload (list API).
type Asset struct {
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

// Image holds binary payload for GET file/thumbnail endpoints.
type Image struct {
	ID       string
	MimeType string
	Data     []byte
}

// Repo abstracts avatar_assets persistence (legacy SQLite layout).
type Repo interface {
	ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]Asset, error)
	GetAvatarAssetByKey(ctx context.Context, assetKey string) (Asset, error)
	GetAvatarImage(ctx context.Context, id string, thumbnail bool) (Image, error)
	CreateAvatarAsset(ctx context.Context, asset Asset, imageData, thumbnailData []byte) (Asset, error)
	UpdateAvatarAssetImages(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error
	SoftDeleteAvatarAsset(ctx context.Context, id string) error
}

// Usecase implements avatar workflows.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs an avatar Usecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

// ListAvatarAssets lists avatar assets with optional scope/workspace/owner filters.
func (uc *Usecase) ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]Asset, error) {
	return uc.repo.ListAvatarAssets(ctx, strings.TrimSpace(scope), strings.TrimSpace(workspaceID), strings.TrimSpace(ownerUserID))
}

// GetAvatarImage returns the image data for a given avatar ID.
func (uc *Usecase) GetAvatarImage(ctx context.Context, id string, thumbnail bool) (Image, error) {
	img, err := uc.repo.GetAvatarImage(ctx, id, thumbnail)
	if err != nil {
		return Image{}, err
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

// UploadAvatar processes and stores an uploaded avatar image.
func (uc *Usecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (Asset, error) {
	if len(data) == 0 {
		return Asset{}, errors.BadRequest("AVATAR", "avatar file is required")
	}
	const max = 2 * 1024 * 1024
	if len(data) > max {
		return Asset{}, errors.BadRequest("AVATAR", "avatar file must be <= 2MB")
	}
	mt := http.DetectContentType(data)
	if mt != "image/png" && mt != "image/jpeg" && mt != "image/webp" && mt != "image/gif" {
		return Asset{}, errors.BadRequest("AVATAR", "unsupported avatar type")
	}
	mainData, thumbData, width, height, outMime, procErr := ProcessAvatarUpload(data, mt)
	if procErr != nil {
		return Asset{}, procErr
	}
	id := newAvatarID()
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "上传头像"
	}
	asset := Asset{
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

// DeleteAvatarAsset soft-deletes an avatar asset.
func (uc *Usecase) DeleteAvatarAsset(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("AVATAR", "avatar id is required")
	}
	return uc.repo.SoftDeleteAvatarAsset(ctx, id)
}
