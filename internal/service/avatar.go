package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/avatar/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// AvatarService implements kratos avatar.v1.
type AvatarService struct {
	v1.UnimplementedAvatarServiceServer

	uc *biz.AvatarUsecase
}

// NewAvatarService constructs AvatarService.
func NewAvatarService(uc *biz.AvatarUsecase) *AvatarService {
	return &AvatarService{uc: uc}
}

// 将 biz.AvatarAsset 转换为 v1.AvatarAsset
func toProtoAvatar(a *biz.AvatarAsset) *v1.AvatarAsset {
	if a == nil {
		return nil
	}
	return &v1.AvatarAsset{
		Id:            a.ID,
		Key:           a.Key,
		Name:          a.Name,
		Description:   a.Description,
		MimeType:      a.MimeType,
		WorkspaceId:   a.WorkspaceID,
		OwnerUserId:   a.OwnerUserID,
		Source:        a.Source,
		IsSystem:      a.IsSystem,
		FileSizeBytes: int32(a.FileSizeBytes),
		WidthPx:       int32(a.WidthPx),
		HeightPx:      int32(a.HeightPx),
		SortOrder:     int32(a.SortOrder),
		CreatedAt:     a.CreatedAt,
	}
}

// ListAvatarAssets implements GET /v1/avatar-assets.
func (s *AvatarService) ListAvatarAssets(ctx context.Context, req *v1.ListAvatarAssetsRequest) (*v1.ListAvatarAssetsResponse, error) {
	items, err := s.uc.ListAvatarAssets(ctx, req.GetScope(), req.GetWorkspaceId(), req.GetOwnerUserId())
	if err != nil {
		return nil, err
	}
	out := &v1.ListAvatarAssetsResponse{Items: make([]*v1.AvatarAsset, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, toProtoAvatar(&items[i]))
	}
	return out, nil
}

// CreateAvatarAsset implements POST /v1/avatar-assets (JSON/protobuf body with raw image bytes; 非 multipart）。
func (s *AvatarService) CreateAvatarAsset(ctx context.Context, req *v1.CreateAvatarAssetRequest) (*v1.AvatarAsset, error) {
	a, err := s.uc.UploadAvatar(ctx, req.GetImageData(), req.GetFilename(), req.GetWorkspaceId(), req.GetOwnerUserId())
	if err != nil {
		return nil, err
	}
	return toProtoAvatar(&a), nil
}

// GetAvatarFile implements GET /v1/avatar-assets/{id}/file.
func (s *AvatarService) GetAvatarFile(ctx context.Context, req *v1.GetAvatarBlobRequest) (*v1.GetAvatarBlobResponse, error) {
	return s.blobResponse(ctx, req.GetId(), false)
}

// GetAvatarThumbnail implements GET /v1/avatar-assets/{id}/thumbnail.
func (s *AvatarService) GetAvatarThumbnail(ctx context.Context, req *v1.GetAvatarBlobRequest) (*v1.GetAvatarBlobResponse, error) {
	return s.blobResponse(ctx, req.GetId(), true)
}

func (s *AvatarService) blobResponse(ctx context.Context, id string, thumbnail bool) (*v1.GetAvatarBlobResponse, error) {
	img, err := s.uc.GetAvatarImage(ctx, id, thumbnail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AVATAR", "avatar not found")
		}
		return nil, err
	}
	return &v1.GetAvatarBlobResponse{MimeType: img.MimeType, Data: img.Data}, nil
}

// DeleteAvatarAsset performs soft-delete.
func (s *AvatarService) DeleteAvatarAsset(ctx context.Context, req *v1.DeleteAvatarAssetRequest) (*emptypb.Empty, error) {
	err := s.uc.DeleteAvatarAsset(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AVATAR", "avatar not found")
		}
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
