package service

import (
	"aranea-agents/internal/team"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	team.ProviderSet,
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
	NewMemoryService,
	NewSystemSettingService,
	NewChatService,
)
