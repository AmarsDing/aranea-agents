package repository

import (
	"arenea/backend/internal/catalog/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) catalogAvatar() *sqlite.AvatarRepository {
	return sqlite.NewAvatarRepository(r.db)
}

func (r *SQLiteRepository) seedAvatarAssets() error {
	return r.catalogAvatar().SeedAvatarAssets()
}

func (r *SQLiteRepository) ListAvatarAssets(scope string, workspaceID string, ownerUserID string) ([]domain.AvatarAsset, error) {
	return r.catalogAvatar().ListAvatarAssets(scope, workspaceID, ownerUserID)
}

func (r *SQLiteRepository) GetAvatarImage(id string, thumbnail bool) (domain.AvatarImage, error) {
	return r.catalogAvatar().GetAvatarImage(id, thumbnail)
}

func (r *SQLiteRepository) CreateAvatarAsset(asset domain.AvatarAsset, imageData []byte, thumbnailData []byte) (domain.AvatarAsset, error) {
	return r.catalogAvatar().CreateAvatarAsset(asset, imageData, thumbnailData)
}
