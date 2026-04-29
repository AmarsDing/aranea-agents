package biz

import "github.com/google/wire"

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewTeamRunEventBroker,
	NewAdminUsecase,
	NewAvatarUsecase,
	NewMemoryUsecase,
	NewAgentUsecase,
	NewTeamUsecase,
	NewAgentCategoryUsecase,
	NewLlmProviderModelUsecase,
	NewHookUsecase,
	NewCronUsecase,
	NewPluginUsecase,
	NewMCPServerUsecase,
	NewSkillUsecase,
	NewSessionUsecase,
	NewToolUsecase,
	NewChannelUsecase,
	NewUsageUsecase,
	NewMonitorUsecase,
)
