package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/team"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	wire.Bind(new(biz.NativeTurnCompressor), new(*SessionCompressor)),
	wire.Bind(new(compress.Compressor), new(*compress.LLMService)),
	NewCompressHTTPClient,
	compress.NewLLMService,
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
	NewSessionCompressor,
)
