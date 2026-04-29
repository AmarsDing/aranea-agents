package service

import "github.com/google/wire"

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewAdminService,
	NewAvatarService,
	NewAgentService,
	NewTeamService,
	NewAgentCategoryService,
	NewLlmProviderModelService,
	NewHookService,
	NewCronService,
	NewPluginService,
	NewMCPServerService,
	NewSkillService,
	NewSessionService,
	NewToolService,
	NewChannelService,
	NewUsageService,
	NewMonitorService,
)
