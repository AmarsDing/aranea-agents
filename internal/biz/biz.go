package biz

import "github.com/google/wire"

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewAdminUsecase,
	NewAvatarUsecase,
	NewMemoryUsecase,
	NewAgentUsecase,
	NewAgentCategoryUsecase,
	NewLlmProviderModelUsecase,
	NewHookUsecase,
	NewMCPServerUsecase,
)
