package biz

import (
	"aranea-agents/internal/biz/avatar"
)

// Re-export avatar types from sub-package for backward compatibility.
type (
	AvatarAsset   = avatar.Asset
	AvatarImage   = avatar.Image
	AvatarRepo    = avatar.Repo
	AvatarUsecase = avatar.Usecase
)

// Re-export avatar constructor for backward compatibility.
var NewAvatarUsecase = avatar.NewUsecase
