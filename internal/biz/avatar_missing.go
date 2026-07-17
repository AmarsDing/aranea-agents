package biz

import (
	"errors"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// isAvatarAssetMissing reports whether GetAvatarAssetByKey (or equivalent)
// indicated the asset does not exist yet. data.avatarRepo returns
// apierror.NotFound; unit mocks may return shared.ErrNotFound.
func isAvatarAssetMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, shared.ErrNotFound) {
		return true
	}
	return apierror.IsCode(err, apierror.CodeNotFound)
}
