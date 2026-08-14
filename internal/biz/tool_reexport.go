package biz

import (
	"context"

	"aranea-agents/internal/biz/tool"
)

type (
	Tool                     = tool.Tool
	ToolCatalogEntry         = tool.ToolCatalogEntry
	ToolPermissions          = tool.ToolPermissions
	ToolUpsertInput          = tool.ToolUpsertInput
	ToolListQuery            = tool.ToolListQuery
	ToolListResult           = tool.ToolListResult
	ToolSummary              = tool.ToolSummary
	ToolInvocation           = tool.ToolInvocation
	ToolInvocationWrite      = tool.ToolInvocationWrite
	ToolInvocationParam      = tool.ToolInvocationParam
	ToolAgentOverride        = tool.ToolAgentOverride
	ToolAgentOverrideInput   = tool.ToolAgentOverrideInput
	ToolRunQuery             = tool.ToolRunQuery
	ToolRunResult            = tool.ToolRunResult
	ToolInvocationAuditWrite = tool.ToolInvocationAuditWrite
	ToolInvocationAudit      = tool.ToolInvocationAudit
	ToolAuditQuery           = tool.ToolAuditQuery
	ToolAuditResult          = tool.ToolAuditResult
	ToolRepo                 = tool.ToolRepo
	ToolReader               = tool.ToolReader
	ToolWriter               = tool.ToolWriter
	ToolInvocationReader     = tool.ToolInvocationReader
	ToolInvocationWriter     = tool.ToolInvocationWriter
	ToolAuditRepo            = tool.ToolAuditRepo
	ToolOverrideReader       = tool.ToolOverrideReader
	ToolOverrideWriter       = tool.ToolOverrideWriter
	ToolSyncer               = tool.ToolSyncer
	ToolRegistryReader       = tool.ToolRegistryReader
	ToolQualityStat          = tool.ToolQualityStat
	ToolQualityStatsReader   = tool.ToolQualityStatsReader
	ToolUsecase              = tool.ToolUsecase
	ToolTestResult           = tool.ToolTestResult
	ToolSettingRepo          = tool.SettingRepo
	ToolGrant                = tool.ToolGrant
	ToolGrantReader          = tool.ToolGrantReader
	ToolGrantWriter          = tool.ToolGrantWriter
	ToolGrantStore           = tool.ToolGrantStore
	WebResearchReadinessFunc = tool.WebResearchReadinessFunc
)

var (
	NewToolUsecase = tool.NewToolUsecase

	NormalizeToolPolicyKey              = tool.NormalizeToolPolicyKey
	PropagateAllowAliases               = tool.PropagateAllowAliases
	PropagateDenyAliases                = tool.PropagateDenyAliases
	LoadWebResearchPlatform             = tool.LoadWebResearchPlatform
	EnrichToolRuntimeFieldsWithPlatform = tool.EnrichToolRuntimeFieldsWithPlatform
	CheckerToReadinessFunc              = tool.CheckerToReadinessFunc
	MergeToolConfigMaps                 = tool.MergeToolConfigMaps
	MergeJSONMapInto                    = tool.MergeJSONMapInto
	ToolRequiresConfirmation            = tool.ToolRequiresConfirmation
	RedactToolConfigJSON                = tool.RedactToolConfigJSON
)

const (
	ToolInvocationSourceEventBus = tool.ToolInvocationSourceEventBus
	ToolInvocationSourceRuntime  = tool.ToolInvocationSourceRuntime
	ToolInvocationSourceMCP      = tool.ToolInvocationSourceMCP
	ToolAuditRetentionDays       = tool.ToolAuditRetentionDays
)

// toolSettingAdapter adapts biz.SystemSettingRepo to tool.SettingRepo.
type toolSettingAdapter struct {
	sys SystemSettingRepo
}

func (a *toolSettingAdapter) GetWebResearch(ctx context.Context) (tool.WebResearchSetting, error) {
	return a.sys.GetWebResearch(ctx)
}

// NewToolSettingRepo wraps a biz.SystemSettingRepo as a tool.SettingRepo.
func NewToolSettingRepo(sys SystemSettingRepo) tool.SettingRepo {
	return &toolSettingAdapter{sys: sys}
}
