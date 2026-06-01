package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizavatar "aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/avatarasset"
	"aranea-agents/internal/data/ent/predicate"

	entsql "entgo.io/ent/dialect/sql"
)

type avatarRepo struct {
	data *Data
}

var _ bizavatar.Repo = (*avatarRepo)(nil)

func NewAvatarRepo(d *Data) biz.AvatarRepo {
	return &avatarRepo{data: d}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func entToBizAvatar(po *ent.AvatarAsset) biz.AvatarAsset {
	if po == nil {
		return biz.AvatarAsset{}
	}
	return biz.AvatarAsset{
		ID:            po.ID,
		Key:           po.AssetKey,
		Name:          po.Name,
		Description:   po.Description,
		MimeType:      po.MimeType,
		WorkspaceID:   po.WorkspaceID,
		OwnerUserID:   po.OwnerUserID,
		Source:        po.Source,
		Category:      po.Category,
		IsSystem:      po.IsSystem,
		FileSizeBytes: po.FileSizeBytes,
		WidthPx:       po.WidthPx,
		HeightPx:      po.HeightPx,
		SortOrder:     po.SortOrder,
		CreatedAt:     po.CreatedAt,
	}
}

func (r *avatarRepo) ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]biz.AvatarAsset, error) {
	preds := []predicate.AvatarAsset{
		avatarasset.DeletedAtEQ(""),
		avatarasset.EnabledEQ(true),
		predicate.AvatarAsset(func(s *entsql.Selector) {
			s.Where(entsql.ExprP(fmt.Sprintf("length(%s.image_data) > 0", avatarasset.Table)))
		}),
	}
	switch scope {
	case "system":
		preds = append(preds, avatarasset.IsSystemEQ(true))
	case "mine":
		preds = append(preds, avatarasset.IsSystemEQ(false))
		if workspaceID != "" {
			preds = append(preds, avatarasset.WorkspaceIDEQ(workspaceID))
		}
		if ownerUserID != "" {
			preds = append(preds, avatarasset.OwnerUserIDEQ(ownerUserID))
		}
	default:
		preds = append(preds, avatarasset.Or(
			avatarasset.IsSystemEQ(true),
			avatarasset.WorkspaceIDEQ(workspaceID),
			avatarasset.OwnerUserIDEQ(ownerUserID),
		))
	}

	rows, err := r.data.Ent().AvatarAsset.Query().
		Where(avatarasset.And(preds...)).
		Order(
			avatarasset.ByIsSystem(entsql.OrderDesc()),
			avatarasset.BySortOrder(),
			avatarasset.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.AvatarAsset, 0, len(rows))
	for _, po := range rows {
		out = append(out, entToBizAvatar(po))
	}
	return out, nil
}

func (r *avatarRepo) GetAvatarAssetByKey(ctx context.Context, assetKey string) (biz.AvatarAsset, error) {
	assetKey = strings.TrimSpace(assetKey)
	if assetKey == "" {
		return biz.AvatarAsset{}, sql.ErrNoRows
	}
	po, err := r.data.Ent().AvatarAsset.Query().
		Where(
			avatarasset.AssetKeyEQ(assetKey),
			avatarasset.DeletedAtEQ(""),
			avatarasset.EnabledEQ(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AvatarAsset{}, sql.ErrNoRows
		}
		return biz.AvatarAsset{}, err
	}
	return entToBizAvatar(po), nil
}

func (r *avatarRepo) GetAvatarImage(ctx context.Context, id string, thumbnail bool) (biz.AvatarImage, error) {
	if id == "" {
		return biz.AvatarImage{}, errors.New("avatar id is required")
	}
	po, err := r.data.Ent().AvatarAsset.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AvatarImage{}, sql.ErrNoRows
		}
		return biz.AvatarImage{}, err
	}
	if po.DeletedAt != "" || !po.Enabled {
		return biz.AvatarImage{}, sql.ErrNoRows
	}
	payload := po.ImageData
	if thumbnail && len(po.ThumbnailData) > 0 {
		payload = po.ThumbnailData
	}
	if len(payload) == 0 {
		payload = po.ImageData
	}
	if len(payload) == 0 {
		return biz.AvatarImage{}, sql.ErrNoRows
	}
	return biz.AvatarImage{ID: po.ID, MimeType: po.MimeType, Data: payload}, nil
}

const avatarPersistSize = 256

func (r *avatarRepo) CreateAvatarAsset(ctx context.Context, asset biz.AvatarAsset, imageData, thumbnailData []byte) (biz.AvatarAsset, error) {
	if asset.ID == "" || asset.Key == "" || asset.Name == "" {
		return biz.AvatarAsset{}, errors.New("id, key and name are required")
	}
	if len(imageData) == 0 {
		return biz.AvatarAsset{}, errors.New("image data is required")
	}
	if asset.MimeType == "" {
		asset.MimeType = "image/png"
	}
	if asset.Source == "" {
		asset.Source = "upload"
	}
	if asset.WidthPx == 0 {
		asset.WidthPx = avatarPersistSize
	}
	if asset.HeightPx == 0 {
		asset.HeightPx = avatarPersistSize
	}
	if asset.FileSizeBytes == 0 {
		asset.FileSizeBytes = len(imageData)
	}
	now := nowRFC3339()
	cr := r.data.Ent().AvatarAsset.Create().
		SetID(asset.ID).
		SetAssetKey(asset.Key).
		SetName(asset.Name).
		SetDescription(asset.Description).
		SetMimeType(asset.MimeType).
		SetWorkspaceID(asset.WorkspaceID).
		SetOwnerUserID(asset.OwnerUserID).
		SetSource(asset.Source).
		SetCategory(asset.Category).
		SetIsSystem(asset.IsSystem).
		SetFileSizeBytes(asset.FileSizeBytes).
		SetWidthPx(asset.WidthPx).
		SetHeightPx(asset.HeightPx).
		SetStatus("active").
		SetEnabled(true).
		SetSortOrder(asset.SortOrder).
		SetConfigJSON("").
		SetMetadataJSON("").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetImageData(imageData)
	if len(thumbnailData) > 0 {
		cr.SetThumbnailData(thumbnailData)
	}
	saved, err := cr.Save(ctx)
	if err != nil {
		return biz.AvatarAsset{}, err
	}
	return entToBizAvatar(saved), nil
}

func (r *avatarRepo) UpdateAvatarAssetImages(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error {
	if id == "" {
		return errors.New("avatar id is required")
	}
	if len(imageData) == 0 {
		return errors.New("image data is required")
	}
	if mime == "" {
		mime = "image/png"
	}
	if width == 0 {
		width = avatarPersistSize
	}
	if height == 0 {
		height = avatarPersistSize
	}
	if fileSize == 0 {
		fileSize = len(imageData)
	}
	now := nowRFC3339()
	up := r.data.Ent().AvatarAsset.UpdateOneID(id).
		SetMimeType(mime).
		SetFileSizeBytes(fileSize).
		SetWidthPx(width).
		SetHeightPx(height).
		SetImageData(imageData).
		SetUpdatedAt(now)
	if len(thumbnailData) > 0 {
		up.SetThumbnailData(thumbnailData)
	}
	_, err := up.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return sql.ErrNoRows
		}
		return err
	}
	return nil
}

func (r *avatarRepo) SoftDeleteAvatarAsset(ctx context.Context, id string) error {
	now := nowRFC3339()
	_, err := r.data.Ent().AvatarAsset.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return sql.ErrNoRows
		}
		return err
	}
	return nil
}
